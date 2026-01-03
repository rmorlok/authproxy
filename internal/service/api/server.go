package admin_api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	"github.com/rmorlok/authproxy/internal/service"
)

func GetGinServer(dm *service.DependencyManager) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := dm.GetConfigRoot()
	logger := dm.GetLogger()
	service := &root.Api
	authService := auth.NewService(
		dm.GetConfig(),
		service,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetEncryptService(),
		dm.GetLogger(),
	)

	server := api_common.GinForService(service)

	corsConfig := root.Api.CorsVal.ToGinCorsConfig(nil)
	if corsConfig != nil {
		logger.Info("Enabling CORS")
		server.Use(cors.New(*corsConfig))
	}

	var healthChecker *gin.Engine
	if service.Port() != service.HealthCheckPort() {
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
			dbChan <- dm.GetDatabase().Ping(ctx)
		}()

		go func() {
			redisChan <- apredis.Ping(ctx, dm.GetRedisClient())
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

	routesConnectors := common_routes.NewConnectorsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetCoreService(),
	)
	routesConnections := common_routes.NewConnectionsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetCoreService(),
		dm.GetHttpf(),
		dm.GetEncryptService(),
		logger,
	)
	routesNamespaces := common_routes.NewNamespacesRoutes(
		dm.GetConfig(),
		authService,
		dm.GetCoreService(),
	)
	routesProxy := common_routes.NewConnectionsProxyRoutes(
		dm.GetConfig(),
		authService,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetCoreService(),
		dm.GetHttpf(),
		dm.GetEncryptService(),
		logger,
	)
	routesTasks := common_routes.NewTaskRoutes(
		dm.GetConfig(),
		authService,
		dm.GetEncryptService(),
		dm.GetAsyncInspector(),
	)
	routesRequestLog := common_routes.NewRequestLogRoutes(
		dm.GetConfig(),
		authService,
		dm.GetRequestLogRetriever(),
	)
	routesActors := common_routes.NewActorsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetHttpf(),
		dm.GetEncryptService(),
		logger,
	)

	api := server.Group("/api/v1")

	routesConnectors.Register(api)
	routesConnections.Register(api)
	routesNamespaces.Register(api)
	routesProxy.Register(api)
	routesTasks.Register(api)
	routesRequestLog.Register(api)
	routesActors.Register(api)

	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	dm := service.NewDependencyManager("api", cfg)
	aplog.SetDefaultLog(dm.GetRootLogger())
	logger := dm.GetLogger()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Close redis connections when we exit
	defer dm.GetRedisClient().Close()

	dm.AutoMigrateAll()

	server, healthChecker, err := GetGinServer(dm)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("running service", "addr", server.Addr)
		err := api_common.RunServer(server, logger)
		if err != nil {
			logger.Error(err.Error())
		}
	}()

	if healthChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("running health checker", "addr", healthChecker.Addr)
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
