package admin_api

import (
	"context"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

func rateKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func rateErrorHandler(c *gin.Context, info ratelimit.Info) {
	c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
}

func GetGinServer(
	cfg config.C,
	db database.DB,
	redis redis.R,
	c _interface.C,
	e encrypt.E,
	logger *slog.Logger,
) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := cfg.GetRoot()
	service := &root.AdminApi
	authService := auth.NewService(cfg, service, db, redis, e, logger)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 300,
	})

	server := api_common.GinForService(service)

	corsConfig := root.AdminApi.CorsVal.ToGinCorsConfig(nil)
	if corsConfig != nil {
		logger.Info("Enabling CORS")
		server.Use(cors.New(*corsConfig))
	}

	var healthChecker *gin.Engine
	if root.Public.Port() != root.Public.HealthCheckPort() {
		healthChecker = api_common.GinForService(service)
	} else {
		healthChecker = server
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "admin-api",
			"message": "pong",
		})
	})

	healthChecker.GET("/healthz", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "admin-api",
			"ok":      true,
		})
	})

	api := server.Group("/api/v1", authService.AdminOnly())
	{
		mw := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
			ErrorHandler: rateErrorHandler,
			KeyFunc:      rateKeyFunc,
		})

		api.GET("/todo", mw, func(c *gin.Context) {})
	}

	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	root := cfg.GetRoot()
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("admin-api")
	logger := logBuilder.Build()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	rs, err := redis.New(context.Background(), cfg, logger)
	if err != nil {
		panic(err)
	}
	defer rs.Close()

	db, err := database.NewConnectionForRoot(root, logger)
	if err != nil {
		panic(err)
	}

	if root.Database.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				database.MigrateMutexKeyName,
				redis.MutexOptionLockFor(root.Database.GetAutoMigrationLockDuration()),
				redis.MutexOptionRetryFor(root.Database.GetAutoMigrationLockDuration()+1*time.Second),
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

	h := httpf.CreateFactory(cfg, rs, logger)
	e := encrypt.NewEncryptService(cfg, db)
	asynqClient := asynq.NewClientFromRedisClient(rs.Client())
	c := connectors.NewConnectorsService(cfg, db, e, rs, h, asynqClient, logger)

	if root.Connectors.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				connectors.MigrateMutexKeyName,
				redis.MutexOptionLockFor(root.Connectors.GetAutoMigrationLockDurationOrDefault()),
				redis.MutexOptionRetryFor(root.Connectors.GetAutoMigrationLockDurationOrDefault()+1*time.Second),
				redis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				redis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := c.MigrateConnectors(context.Background()); err != nil {
				panic(err)
			}
		}()
	}

	server, healthChecker, err := GetGinServer(cfg, db, rs, c, e, logger)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := api_common.RunServer(server, logger)
		if err != nil {
			logger.Error(err.Error())
		}
	}()

	if healthChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := api_common.RunServer(healthChecker, logger)
			if err != nil {
				logger.Error(err.Error())
			}
		}()
	}

	wg.Wait()

	logger.Info("Admin API shutting down")
	defer logger.Info("Admin API shutdown complete")
}
