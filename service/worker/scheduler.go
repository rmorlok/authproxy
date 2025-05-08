package worker

import (
	"context"
	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const mutexLockTime = 2 * time.Minute

type scheduler struct {
	redis               redis.R
	healthCheckFunc     func(isScheduler bool, err error)
	oauth2TaskRegistrar oauth2.TaskRegistrar
	mtx                 sync.Mutex
	mgr                 *asynq.PeriodicTaskManager
	wg                  sync.WaitGroup
	done                chan struct{}
	rsMtx               redis.Mutex
	logger              *slog.Logger
}

func newScheduler(rs redis.R, hc func(isScheduler bool, err error), tr oauth2.TaskRegistrar, l *slog.Logger) *scheduler {
	return &scheduler{
		redis:               rs,
		healthCheckFunc:     hc,
		oauth2TaskRegistrar: tr,
		logger:              l,
		done:                make(chan struct{}),
		rsMtx: rs.NewMutex("worker:scheduler_master",
			redis.MutexOptionLockFor(mutexLockTime),
			redis.MutexOptionDetailedLockMetadata(),
		),
	}
}

func (s *scheduler) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	configs := make([]*asynq.PeriodicTaskConfig, 0)
	configs = append(configs, s.oauth2TaskRegistrar.GetCronTasks()...)
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
			RedisUniversalClient:       s.redis.Client(),
			PeriodicTaskConfigProvider: s,
			SyncInterval:               10 * time.Second,
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
	s.logger.Info("Scheduler is running")

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.done:
				s.logger.Info("Shutting down scheduler")
				s.shutdown()
				return
			case <-time.After(mutexLockTime / 2):
				s.logger.Debug("Extending scheduler ownership lock")
				err = s.rsMtx.Extend(ctx, mutexLockTime)
				if err != nil {
					if s.mgr != nil {
						s.logger.Info("Shutting down scheduler due to failure to extend the scheduler ownership lock")
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
		s.logger.Info("Async scheduler shutdown complete")
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
			s.logger.Info("Received termination signal")
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
			s.logger.Info("Shutting down scheduler watchdog")
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
				} else if !redis.MutexIsErrNotObtained(err) {
					s.shutdown()
					s.healthCheckFunc(false, err)
					if lastErr == nil {
						s.logger.Error(
							"Failed to obtain lock for scheduler",
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
