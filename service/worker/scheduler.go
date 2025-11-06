package worker

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/apredis"
)

const mutexLockTime = 2 * time.Minute

type CronRegistrar interface {
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

type scheduler struct {
	r               apredis.Client
	healthCheckFunc func(isScheduler bool, err error)
	registrars      []CronRegistrar
	mtx             sync.Mutex
	mgr             *asynq.PeriodicTaskManager
	wg              sync.WaitGroup
	done            chan struct{}
	rsMtx           apredis.Mutex
	logger          *slog.Logger
	syncInterval    time.Duration
}

func newScheduler(r apredis.Client, hc func(isScheduler bool, err error), l *slog.Logger, syncInterval time.Duration) *scheduler {
	return &scheduler{
		r:               r,
		healthCheckFunc: hc,
		logger:          l,
		done:            make(chan struct{}),
		rsMtx: apredis.NewMutex(r, "worker:scheduler_master",
			apredis.MutexOptionLockFor(mutexLockTime),
			apredis.MutexOptionDetailedLockMetadata(),
		),
		syncInterval: syncInterval,
	}
}

func (s *scheduler) addRegistrar(cr CronRegistrar) *scheduler {
	s.registrars = append(s.registrars, cr)
	return s
}

func (s *scheduler) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	configs := make([]*asynq.PeriodicTaskConfig, 0)

	// This should be updated to handler errors as many of these will come from database records...
	for _, r := range s.registrars {
		configs = append(configs, r.GetCronTasks()...)
	}

	return configs, nil
}

func (s *scheduler) start(ctx context.Context) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.mgr != nil {
		return nil
	}

	s.logger.Info("Obtained lock for scheduler")
	s.healthCheckFunc(true, nil)

	var err error
	s.mgr, err = asynq.NewPeriodicTaskManager(
		asynq.PeriodicTaskManagerOpts{
			RedisUniversalClient:       s.r,
			PeriodicTaskConfigProvider: s,
			SyncInterval:               s.syncInterval,
			SchedulerOpts: &asynq.SchedulerOpts{
				Logger:   &asyncLogger{inner: aplog.NewBuilder(s.logger).WithComponent("asynq-scheduler").Build()},
				LogLevel: asynq.InfoLevel,
			},
		},
	)

	if err != nil {
		return errors.Wrap(err, "error creating periodic task manager")
	}

	err = s.mgr.Start()

	if err != nil {
		s.healthCheckFunc(false, err)
		return err
	}
	s.healthCheckFunc(true, err)
	s.logger.Info("scheduler is running")

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.done:
				s.logger.Info("shutting down scheduler")
				s.shutdown()
				return
			case <-time.After(mutexLockTime / 2):
				s.logger.Debug("extending scheduler ownership lock")
				err = s.rsMtx.Extend(ctx, mutexLockTime)
				if err != nil {
					if s.mgr != nil {
						s.logger.Info("shutting down scheduler due to failure to extend the scheduler ownership lock")
						s.shutdown()
					}
					s.healthCheckFunc(false, nil)
					return
				}
				s.healthCheckFunc(true, nil)
			}
		}
	}()

	return nil
}

func (s *scheduler) shutdown() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.mgr != nil {
		s.mgr.Shutdown()
		s.logger.Info("async scheduler shutdown complete")
		s.mgr = nil
	}
}

func (s *scheduler) Run() error {
	ctx := context.Background()
	defer s.wg.Wait()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-sigChan:
			s.logger.Info("received termination signal")
			close(s.done)
			return
		case <-s.done:
			return
		}
	}()

	var lastErr error

	for {
		select {
		case <-s.done:
			s.logger.Info("shutting down scheduler watchdog")
			s.shutdown()
			return nil
		default:
			if s.mgr == nil {
				err := s.rsMtx.Lock(ctx)
				if err == nil {
					lastErr = nil
					err = s.start(ctx)
					if err != nil {
						s.shutdown()
						s.healthCheckFunc(false, err)
						s.rsMtx.Unlock(ctx)
						return err
					}
				} else if !apredis.MutexIsErrNotObtained(err) {
					s.shutdown()
					s.healthCheckFunc(false, err)
					if lastErr == nil {
						s.logger.Error(
							"failed to obtain lock for scheduler",
							"err", err)
						lastErr = err
					}
				} else {
					s.healthCheckFunc(false, nil)
				}
			}

			time.Sleep(300 * time.Millisecond)
		}
	}
}
