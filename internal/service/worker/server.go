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
	"github.com/rmorlok/authproxy/internal/apasynq"
	authSync "github.com/rmorlok/authproxy/internal/apauth/tasks"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	dbTasks "github.com/rmorlok/authproxy/internal/database/tasks"
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

	// Initialise telemetry early; flush last so other shutdowns can still emit.
	dm.GetTelemetry()
	defer dm.ShutdownTelemetry()

	workerConfig := cfg.GetRoot().Worker
	router := apgin.ForService(&workerConfig, logger, cfg.IsDebugMode(),
		apgin.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry, dm.GetServiceId()))

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

	dm.RegisterDatabasePing()
	dm.RegisterRedisPing()
	dm.RegisterAsynqClientPing()
	dm.RegisterLogStoragePing()
	dm.RegisterPing("asynqServer", func(ctx context.Context) bool {
		return asyncRunning && !asyncHasError
	})

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		results, allOk := dm.RunPings(ctx)
		status := http.StatusOK
		if !allOk {
			status = http.StatusServiceUnavailable
		}

		response := gin.H{"service": "worker", "ok": allOk, "asyncIsScheduler": asyncIsScheduler}
		for k, v := range results {
			response[k] = v
		}
		c.PureJSON(status, response)
	})

	dm.AutoMigrateAll()
	defer dm.GetEncryptService().Shutdown()

	ctx := context.Background()

	// Build the asynq telemetry surface (middleware + scheduler-sync
	// wrapper + queue-depth gauge). Safe to call when telemetry is
	// disabled — every entry point is a no-op in that case.
	asynqTel, err := apasynq.NewTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry, dm.GetAsyncInspector())
	if err != nil {
		log.Fatalf("failed to construct asynq telemetry: %v", err)
	}

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

	// Register the queue-depth observable gauge for every queue the
	// asynq server is configured to consume. The returned stop function
	// unregisters the callback on shutdown.
	stopQueueGauge, err := asynqTel.StartQueueDepthGauge([]string{"default"})
	if err != nil {
		log.Fatalf("failed to start queue depth gauge: %v", err)
	}
	defer stopQueueGauge()

	mux := asynq.NewServeMux()
	// Telemetry middleware wraps every handler with a span + duration
	// histogram observation. Identity wrapper when telemetry is disabled.
	mux.Use(asynqTel.Middleware())

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

	encryptTaskHandler := encrypt.NewEncryptServiceTaskHandler(
		dm.GetConfig(),
		dm.GetDatabase(),
		dm.GetEncryptService(),
		dm.GetRedisClient(),
		logger,
	)
	encryptTaskHandler.RegisterTasks(mux)

	dbTaskHandler := dbTasks.NewTaskHandler(
		cfg,
		dm.GetDatabase(),
		logger,
	)
	dbTaskHandler.RegisterTasks(mux)

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
		asynqTel,
	).
		addRegistrar(oauth2TaskHandler).
		addRegistrar(dm.GetCoreService()).
		addRegistrar(adminSyncTaskHandler).
		addRegistrar(encryptTaskHandler).
		addRegistrar(dbTaskHandler)

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
		if err := apgin.RunServer(httpServer, logger); err != nil {
			log.Fatalf("could not run gin server: %v", err)
		}
		logger.Info("Gin shutdown complete")
	}()

	wg.Wait()
	logger.Info("Worker shutting down")
	defer logger.Info("Worker shutdown complete")
}
