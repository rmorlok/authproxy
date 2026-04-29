package admin_api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	"github.com/rmorlok/authproxy/internal/service"
	_ "github.com/rmorlok/authproxy/internal/service/api/swagger"
	api_swagger "github.com/rmorlok/authproxy/internal/service/api/swagger"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

	server := apgin.ForService(service, logger, dm.GetConfig().IsDebugMode())

	corsConfig := root.Api.CorsVal.ToGinCorsConfig(nil)
	if corsConfig != nil {
		server.Use(apgin.NewCorsMiddleware(*corsConfig, logger))
	}

	// Swagger documentation endpoint
	swaggerHost := service.GetBaseUrl()
	swaggerHost = strings.TrimPrefix(swaggerHost, "https://")
	swaggerHost = strings.TrimPrefix(swaggerHost, "http://")
	api_swagger.SwaggerInfoApi.Host = swaggerHost
	api_swagger.SwaggerInfoApi.InfoInstanceName = "api"
	server.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	server.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.InstanceName("Api"),
	))

	var healthChecker *gin.Engine
	if service.Port() != service.HealthCheckPort() {
		healthChecker = apgin.ForService(service, logger, dm.GetConfig().IsDebugMode())
	} else {
		healthChecker = server
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "api",
			"message": "pong",
		})
	})

	dm.RegisterDatabasePing()
	dm.RegisterRedisPing()
	dm.RegisterLogStoragePing()

	healthChecker.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		results, allOk := dm.RunPings(ctx)
		status := http.StatusOK
		if !allOk {
			status = http.StatusServiceUnavailable
		}

		response := gin.H{"service": "api", "ok": allOk}
		for k, v := range results {
			response[k] = v
		}
		c.PureJSON(status, response)
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
		dm.GetLogStorageService(),
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

	defer dm.GetEncryptService().Shutdown()

	server, healthChecker, err := GetGinServer(dm)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("running service", "addr", server.Addr)
		err := apgin.RunServer(server, logger)
		if err != nil {
			logger.Error(err.Error(), "error", err)
		}
	}()

	if healthChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("running health checker", "addr", healthChecker.Addr)
			err := apgin.RunServer(healthChecker, logger)
			if err != nil {
				logger.Error(err.Error(), "error", err)
			}
		}()
	}

	wg.Wait()
	logger.Info("API shutting down")
	defer logger.Info("API shutdown complete")
}
