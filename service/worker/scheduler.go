package worker

import (
	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type scheduler struct {
	redis               redis.R
	healthCheckFunc     func(isScheduler bool, err error)
	oauth2TaskRegistrar oauth2.TaskRegistrar
}

func (s *scheduler) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	configs := make([]*asynq.PeriodicTaskConfig, 0)
	configs = append(configs, s.oauth2TaskRegistrar.GetCronTasks()...)
	return configs, nil
}

const mutexLockTime = 2 * time.Minute

func (s *scheduler) getMutex() redis.Mutex {
	return s.redis.NewMutex("worker:scheduler_master",
		redis.MutexOptionLockFor(mutexLockTime),
		redis.MutexOptionDetailedLockMetadata(),
	)
}

func (s *scheduler) runScheduler(ctx context.Context) error {
	mgr, err := asynq.NewPeriodicTaskManager(
		asynq.PeriodicTaskManagerOpts{
			RedisUniversalClient:       s.redis.Client(),
			PeriodicTaskConfigProvider: s,
			SyncInterval:               10 * time.Second,
		},
	)

	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mgrDone chan struct{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err = mgr.Run()
		mgrDone <- struct{}{}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-mgrDone:
				close(mgrDone)
				return
			case <-ctx.Done():
				mgr.Shutdown()
				return
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	wg.Wait()

	return err
}

func (s *scheduler) Run(ctx context.Context) error {
	m := s.getMutex()

	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Create a done channel to handle cleanup
	done := make(chan struct{})
	runReturning := make(chan struct{})
	defer close(runReturning)

	// Start a goroutine to handle signals
	go func() {
		select {
		case <-sigChan:
			println("Received termination signal")
			close(done)
			return
		case <-ctx.Done():
			close(done)
			return
		case <-runReturning:
			println("shutting down monitor")
			return
		}
	}()

	for {
		select {
		case <-done:
			println("Shutting down scheduler watchdog")
			return nil
		default:
			err := m.Lock(ctx)
			if err == nil {
				defer m.Unlock(ctx)
				println("Obtained lock for scheduler")
				s.healthCheckFunc(true, nil)

				// We have the lock and are thus the scheduler
				cancelCtx, cancel := context.WithCancel(ctx)

				var wg sync.WaitGroup
				var schedDone chan struct{}

				var runSchedulerError error
				wg.Add(1)
				go func() {
					defer wg.Done()
					println("Scheduler is running")
					runSchedulerError = s.runScheduler(cancelCtx)
					close(schedDone)
					s.healthCheckFunc(false, runSchedulerError)
					println("Scheduler has stopped")
				}()

				var extendLockError error
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-schedDone:
							return
						case <-ctx.Done():
							return
						case <-time.After(mutexLockTime / 2):
							extendLockError = m.Extend(ctx, mutexLockTime)
							if extendLockError != nil {
								cancel()
								return
							}
							s.healthCheckFunc(true, nil)
						}
					}
				}()

				wg.Wait()

				if extendLockError != nil {
					return extendLockError
				}

				return runSchedulerError
			}

			if !redis.MutexIsErrNotObtained(err) {
				s.healthCheckFunc(false, err)
				return errors.Wrap(err, "error while attaining lock for scheduler")
			} else {
				s.healthCheckFunc(false, nil)
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}
