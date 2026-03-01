package worker

import (
	"context"
	context2 "context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	authSync "github.com/rmorlok/authproxy/internal/apauth/tasks"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/service"
)

func Serve(cfg config.C) {
	dm := service.NewDependencyManager("worker", cfg)
	aplog.SetDefaultLog(dm.GetRootLogger())
	logger := dm.GetLogger()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	workerConfig := cfg.GetRoot().Worker
	router := api_common.GinForService(&workerConfig, logger, cfg.IsDebugMode())

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

	dm.GetAsyncClient().Ping()

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		dbChan := make(chan bool, 1)
		redisChan := make(chan bool, 1)
		asynqClientChan := make(chan bool, 1)

		go func() {
			dbChan <- dm.GetDatabase().Ping(ctx)
		}()

		go func() {
			redisChan <- apredis.Ping(ctx, dm.GetRedisClient())
		}()

		go func() {
			if err := dm.GetAsyncClient().Ping(); err != nil {
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

	dm.AutoMigrateAll()

	ctx := context.Background()

	srv := asynq.NewServerFromRedisClient(
		dm.GetRedisClient(),
		asynq.Config{
			HealthCheckFunc: asyncHealthChecker,
			Concurrency:     workerConfig.GetConcurrency(context.Background()),
			BaseContext: func() context2.Context {
				return ctx
			},
			Logger:   &asyncLogger{inner: dm.GetLogBuilder().WithComponent("asynq").Build()},
			LogLevel: asynq.InfoLevel,
			Queues: map[string]int{
				"default": 5,
			},
		},
	)

	mux := asynq.NewServeMux()

	oauth2TaskHandler := oauth2.NewTaskHandler(
		cfg,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetCoreService(),
		dm.GetAsyncClient(),
		dm.GetHttpf(),
		dm.GetEncryptService(),
		logger,
	)
	oauth2TaskHandler.RegisterTasks(mux)
	dm.GetCoreService().RegisterTasks(mux)

	adminSyncTaskHandler := authSync.NewTaskHandler(
		cfg,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetEncryptService(),
		logger,
	)
	adminSyncTaskHandler.RegisterTasks(mux)

	reencryptTaskHandler := encrypt.NewReencryptTaskHandler(
		dm.GetDatabase(),
		dm.GetEncryptService(),
		logger,
	)
	reencryptTaskHandler.RegisterTasks(mux)

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
		dm.GetRedisClient(),
		asyncSchedulerHealthChecker,
		dm.GetLogBuilder().WithComponent("scheduler").Build(),
		workerConfig.GetCronSyncInterval(),
	).
		addRegistrar(oauth2TaskHandler).
		addRegistrar(dm.GetCoreService()).
		addRegistrar(adminSyncTaskHandler)

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
		httpServer := &http.Server{
			Addr:    fmt.Sprintf(":%d", workerConfig.HealthCheckPort()),
			Handler: router,
		}
		if err := api_common.RunServer(httpServer, logger); err != nil {
			log.Fatalf("could not run gin server: %v", err)
		}
		logger.Info("Gin shutdown complete")
	}()

	wg.Wait()
	logger.Info("Worker shutting down")
	defer logger.Info("Worker shutdown complete")
}
