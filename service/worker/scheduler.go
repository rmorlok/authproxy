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

func (s *scheduler) Run(ctx context.Context) error {
	m := s.getMutex()

	var mgr *asynq.PeriodicTaskManager
	var mgrMutex sync.Mutex

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
			func() {
				mgrMutex.Lock()
				defer mgrMutex.Unlock()
				if mgr != nil {
					mgr.Shutdown()
					mgr = nil
				}
			}()
			return nil
		default:
			err := m.Lock(ctx)
			if err == nil {
				defer m.Unlock(ctx)
				println("Obtained lock for scheduler")
				s.healthCheckFunc(true, nil)

				var wg sync.WaitGroup

				func() {
					mgrMutex.Lock()
					defer mgrMutex.Unlock()

					mgr, err = asynq.NewPeriodicTaskManager(
						asynq.PeriodicTaskManagerOpts{
							RedisUniversalClient:       s.redis.Client(),
							PeriodicTaskConfigProvider: s,
							SyncInterval:               10 * time.Second,
						},
					)
				}()

				if err != nil {
					return errors.Wrap(err, "error creating periodic task manager")
				}

				runSchedulerError := mgr.Start()
				if runSchedulerError != nil {
					s.healthCheckFunc(false, runSchedulerError)
					return runSchedulerError
				}
				s.healthCheckFunc(true, runSchedulerError)
				println("Scheduler is running")

				var extendLockError error
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-done:
							println("Shutting down scheduler")
							func() {
								mgrMutex.Lock()
								defer mgrMutex.Unlock()
								if mgr != nil {
									mgr.Shutdown()
									mgr = nil
								}
							}()
							return
						case <-ctx.Done():
							func() {
								mgrMutex.Lock()
								defer mgrMutex.Unlock()
								if mgr != nil {
									mgr.Shutdown()
									mgr = nil
								}
							}()
							return
						case <-time.After(mutexLockTime / 2):
							extendLockError = m.Extend(ctx, mutexLockTime)
							if extendLockError != nil {
								func() {
									mgrMutex.Lock()
									defer mgrMutex.Unlock()
									if mgr != nil {
										mgr.Shutdown()
										mgr = nil
									}
								}()
								s.healthCheckFunc(false, nil)
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
				func() {
					mgrMutex.Lock()
					defer mgrMutex.Unlock()
					if mgr != nil {
						mgr.Shutdown()
						mgr = nil
					}
				}()
				s.healthCheckFunc(false, err)
				return errors.Wrap(err, "error while attaining lock for scheduler")
			} else {
				s.healthCheckFunc(false, nil)
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}
