package admin_api

import (
	"context"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/apasynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	common_routes "github.com/rmorlok/authproxy/routes"
	"github.com/rmorlok/authproxy/service/public/routes"
	"github.com/rmorlok/authproxy/util"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

func GetCorsConfig(cfg config.C) *cors.Config {
	root := cfg.GetRoot()
	public := root.Public

	var baseConfig *cors.Config
	if root.Marketplace != nil &&
		root.Marketplace.BaseUrl != nil &&
		root.Marketplace.BaseUrl.HasValue(context.Background()) {

		// If marketplace is configured as an external service, allow CORS to that host
		marketplaceUrl := util.Must(root.Marketplace.BaseUrl.GetValue(context.Background()))
		baseConfig = &cors.Config{
			AllowOrigins:     []string{marketplaceUrl},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "HEAD"},
			AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Cookie", "X-Xsrf-Token"},
			ExposeHeaders:    []string{"Cache-Control", "Content-Language", "Content-Length", "Content-Type", "Expires", "Last-Modified", "Pragma", "X-Xsrf-Token"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}
	}

	return public.CorsVal.ToGinCorsConfig(baseConfig)
}

func GetGinServer(
	cfg config.C,
	db database.DB,
	redis redis.R,
	c connectors.C,
	asynqInspector apasynq.Inspector,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := cfg.GetRoot()
	service := &root.Public
	authService := auth.NewService(cfg, service, db, redis, encrypt, logger)

	server := api_common.GinForService(service)

	var healthChecker *gin.Engine
	if service.Port() != service.HealthCheckPort() {
		healthChecker = api_common.GinForService(service)
	} else {
		healthChecker = server
	}

	corsConfig := GetCorsConfig(cfg)
	if corsConfig != nil {
		logger.Info("Enabling CORS")
		server.Use(cors.New(*corsConfig))
	}

	if service.StaticVal != nil {
		// Static content
		server.Use(static.Serve(service.StaticVal.MountAtPath, static.LocalFile(service.StaticVal.ServeFromPath, true)))
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "public",
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

	routesError := routes.NewErrorRoutes(cfg)
	routesError.Register(server)

	routesOauth2 := routes.NewOauth2Routes(cfg, authService, db, redis, c, httpf, encrypt, logger)
	routesOauth2.Register(server)

	var api *gin.RouterGroup
	if service.EnableMarketplaceApis() {
		if api == nil {
			api = server.Group("/api/v1")
		}

		routesSession := routes.NewSessionRoutes(cfg, authService, db, redis, httpf, encrypt, logger)
		routesSession.Register(api)

		routesConnectors := common_routes.NewConnectorsRoutes(cfg, authService, c)
		routesConnectors.Register(api)

		routesConnections := common_routes.NewConnectionsRoutes(cfg, authService, db, redis, c, httpf, encrypt, logger)
		routesConnections.Register(api)

		routesTasks := common_routes.NewTaskRoutes(cfg, authService, encrypt, asynqInspector)
		routesTasks.Register(api)
	}

	if service.EnableProxy() {
		if api == nil {
			api = server.Group("/api/v1")
		}

		proxyRoutes := common_routes.NewConnectionsProxyRoutes(cfg, authService, db, redis, c, httpf, encrypt, logger)
		proxyRoutes.Register(api)
	}
	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	root := cfg.GetRoot()
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("public")
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

	h := httpf.CreateFactory(cfg, rs)
	e := encrypt.NewEncryptService(cfg, db)
	asynqClient := asynq.NewClientFromRedisClient(rs.Client())
	asynqInspector := asynq.NewInspectorFromRedisClient(rs.Client())
	c := connectors.NewConnectorsService(cfg, db, e, asynqClient, logger)

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
	logger.Info("Public shutting down")
	defer logger.Info("Public shutdown complete")
}
