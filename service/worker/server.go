package worker

import (
	"context"
	context2 "context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"log"
	"net/http"
	"sync"
	"time"
)

func Serve(cfg config.C) {
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("worker")
	logger := logBuilder.Build()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	rs, err := redis.New(context.Background(), cfg, logger)
	if err != nil {
		panic(err)
	}
	defer rs.Close()

	db, err := database.NewConnectionForRoot(cfg.GetRoot(), logger)
	if err != nil {
		panic(err)
	}

	if cfg.GetRoot().Database.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				database.MigrateMutexKeyName,
				redis.MutexOptionLockFor(cfg.GetRoot().Database.GetAutoMigrationLockDuration()),
				redis.MutexOptionRetryFor(cfg.GetRoot().Database.GetAutoMigrationLockDuration()+1*time.Second),
				redis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				redis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := db.Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}

	h := httpf.CreateFactory(cfg, rs)
	e := encrypt.NewEncryptService(cfg, db)

	workerConfig := cfg.GetRoot().Worker
	router := api_common.GinForService(&workerConfig)

	router.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "worker",
			"message": "pong",
		})
	})

	asyncHasError := false
	asyncRunning := false
	asyncIsScheduler := false
	asyncHealthChecker := func(err error) {
		asyncHasError = asyncHasError || err != nil
	}

	asyncSchedulerHealthChecker := func(isScheduler bool, err error) {
		asyncHasError = asyncHasError || err != nil
		asyncIsScheduler = isScheduler
	}

	asynqClient := asynq.NewClientFromRedisClient(rs.Client())
	// defer asynqClient.Close() // Do no close the async connection because it is a shared redis connection

	asynqClient.Ping()

	c := connectors.NewConnectorsService(cfg, db, e, asynqClient, logger)

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		dbChan := make(chan bool, 1)
		redisChan := make(chan bool, 1)
		asynqClientChan := make(chan bool, 1)

		go func() {
			dbChan <- db.Ping(ctx)
		}()

		go func() {
			redisChan <- rs.Ping(ctx)
		}()

		go func() {
			if err := asynqClient.Ping(); err != nil {
				asynqClientChan <- false
			} else {
				asynqClientChan <- true
			}
		}()

		dbOk := <-dbChan
		redisOk := <-redisChan
		asyncClientOk := <-asynqClientChan
		everythingOk := dbOk && redisOk && asyncRunning && !asyncHasError && asyncClientOk
		status := http.StatusOK
		if !everythingOk {
			status = http.StatusServiceUnavailable
		}

		c.PureJSON(status, gin.H{
			"service":          "worker",
			"db":               dbOk,
			"redis":            redisOk,
			"asynqServer":      asyncRunning && !asyncHasError,
			"asynqClient":      asyncClientOk,
			"asyncIsScheduler": asyncIsScheduler,
			"ok":               everythingOk,
		})
	})

	ctx := context.Background()

	srv := asynq.NewServerFromRedisClient(
		rs.Client(),
		asynq.Config{
			HealthCheckFunc: asyncHealthChecker,
			Concurrency:     workerConfig.GetConcurrency(context.Background()),
			BaseContext: func() context2.Context {
				return ctx
			},
			Logger:   &asyncLogger{inner: logBuilder.WithComponent("asynq").Build()},
			LogLevel: asynq.InfoLevel,
			Queues: map[string]int{
				"default": 5,
			},
		},
	)

	logBuilder.Build().Info("TEST LOG")

	mux := asynq.NewServeMux()

	oauth2TaskHandler := oauth2.NewTaskHandler(cfg, db, rs, c, asynqClient, h, e, logger)
	oauth2TaskHandler.RegisterTasks(mux)

	connectorsService := connectors.NewConnectorsService(cfg, db, e, asynqClient, logger)
	connectorsService.RegisterTasks(mux)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(mux); err != nil {
			asyncHasError = true
			log.Fatalf("could not run async server: %v", err)
		}
		asyncRunning = false
		logger.Info("Async worker shutdown complete")
	}()

	scheduler := newScheduler(
		rs,
		asyncSchedulerHealthChecker,
		logBuilder.WithComponent("scheduler").Build(),
	).
		addRegistrar(oauth2TaskHandler).
		addRegistrar(connectorsService)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := scheduler.Run(); err != nil {
			asyncHasError = true
			log.Fatalf("could not run scheduler: %v", err)
		}
		asyncIsScheduler = false
		logger.Info("Async scheduler shutdown complete")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := api_common.RunGin(router, fmt.Sprintf(":%d", cfg.GetRoot().Worker.HealthCheckPort()), logger); err != nil {
			log.Fatalf("could not run gin server: %v", err)
		}
		logger.Info("Gin shutdown complete")
	}()

	wg.Wait()
	logger.Info("Worker shutting down")
	defer logger.Info("Worker shutdown complete")
}
