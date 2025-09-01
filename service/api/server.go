package admin_api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apasynq"
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
	"github.com/rmorlok/authproxy/request_log"
	common_routes "github.com/rmorlok/authproxy/routes"
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
	asynqInspector apasynq.Inspector,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := cfg.GetRoot()
	service := &root.Api
	authService := auth.NewService(cfg, service, db, redis, encrypt, logger)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 10_000,
	})

	server := api_common.GinForService(service)

	corsConfig := root.Api.CorsVal.ToGinCorsConfig(nil)
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
			"service": "api",
			"message": "pong",
		})
	})

	healthChecker.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		dbChan := make(chan bool, 1)
		redisChan := make(chan bool, 1)

		go func() {
			dbChan <- db.Ping(ctx)
		}()

		go func() {
			redisChan <- redis.Ping(ctx)
		}()

		dbOk := <-dbChan
		redisOk := <-redisChan
		everythingOk := dbOk && redisOk
		status := http.StatusOK
		if !everythingOk {
			status = http.StatusServiceUnavailable
		}

		c.PureJSON(status, gin.H{
			"service": "api",
			"db":      dbOk,
			"redis":   redisOk,
			"ok":      everythingOk,
		})
	})

	rl := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
		ErrorHandler: rateErrorHandler,
		KeyFunc:      rateKeyFunc,
	})

	routesConnectors := common_routes.NewConnectorsRoutes(cfg, authService, c)
	routesConnections := common_routes.NewConnectionsRoutes(cfg, authService, db, redis, c, httpf, encrypt, logger)
	routesProxy := common_routes.NewConnectionsProxyRoutes(cfg, authService, db, redis, c, httpf, encrypt, logger)
	routesTasks := common_routes.NewTaskRoutes(cfg, authService, encrypt, asynqInspector)
	routesRequestLog := common_routes.NewRequestLogRoutes(cfg, authService, c, db, redis, encrypt)

	api := server.Group("/api/v1", rl)

	routesConnectors.Register(api)
	routesConnections.Register(api)
	routesProxy.Register(api)
	routesTasks.Register(api)
	routesRequestLog.Register(api)

	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	root := cfg.GetRoot()
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("api")
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
				panic(errors.Wrap(err, "failed to establish lock for database migration"))
			}
			defer m.Unlock(context.Background())

			if err := db.Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}

	h := httpf.CreateFactory(cfg, rs, logger)

	if root.HttpLogging.GetAutoMigrate() {
		err := request_log.Migrate(context.Background(), rs, logger)
		if err != nil {
			panic(err)
		}
	}

	e := encrypt.NewEncryptService(cfg, db)
	asynqClient := asynq.NewClientFromRedisClient(rs.Client())
	asynqInspector := asynq.NewInspectorFromRedisClient(rs.Client())

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

	server, healthChecker, err := GetGinServer(cfg, db, rs, c, asynqInspector, h, e, logger)
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
	logger.Info("API shutting down")
	defer logger.Info("API shutdown complete")
}
