package admin_api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth"
	"github.com/rmorlok/authproxy/internal/config"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/rmorlok/authproxy/internal/service/public/routes"
	"github.com/rmorlok/authproxy/internal/util"
)

func GetCorsConfig(cfg config.C) *cors.Config {
	root := cfg.GetRoot()
	service := root.Public

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

	return service.CorsVal.ToGinCorsConfig(baseConfig)
}

func GetGinServer(dm *service.DependencyManager) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := dm.GetConfigRoot()
	service := &root.Public
	logger := dm.GetLogger()
	authService := auth.NewService(
		dm.GetConfig(),
		service,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetEncryptService(),
		dm.GetLogger(),
	)

	server := api_common.GinForService(service)

	var healthChecker *gin.Engine
	if service.Port() != service.HealthCheckPort() {
		healthChecker = api_common.GinForService(service)
	} else {
		healthChecker = server
	}

	corsConfig := GetCorsConfig(dm.GetConfig())
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

	routesError := common_routes.NewErrorRoutes(dm.GetConfig())
	routesError.Register(server)

	routesOauth2 := routes.NewOauth2Routes(
		dm.GetConfig(),
		authService,
		dm.GetDatabase(),
		dm.GetRedisClient(),
		dm.GetCoreService(),
		dm.GetHttpf(),
		dm.GetEncryptService(),
		logger,
	)
	routesOauth2.Register(server)

	var api *gin.RouterGroup
	if service.EnableMarketplaceApis() {
		if api == nil {
			api = server.Group("/api/v1")
		}

		routesSession := common_routes.NewSessionRoutes(
			dm.GetConfig(),
			&root.HostApplication,
			authService,
			dm.GetDatabase(),
			dm.GetRedisClient(),
			dm.GetHttpf(),
			dm.GetEncryptService(),
			logger,
		)
		routesSession.Register(api)

		routesConnectors := common_routes.NewConnectorsRoutes(
			dm.GetConfig(),
			authService, dm.GetCoreService(),
		)
		routesConnectors.Register(api)

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
		routesConnections.Register(api)

		routesTasks := common_routes.NewTaskRoutes(
			dm.GetConfig(),
			authService,
			dm.GetEncryptService(),
			dm.GetAsyncInspector(),
		)
		routesTasks.Register(api)
	}

	if service.EnableProxy() {
		if api == nil {
			api = server.Group("/api/v1")
		}

		proxyRoutes := common_routes.NewConnectionsProxyRoutes(
			dm.GetConfig(),
			authService,
			dm.GetDatabase(),
			dm.GetRedisClient(),
			dm.GetCoreService(),
			dm.GetHttpf(),
			dm.GetEncryptService(),
			logger,
		)
		proxyRoutes.Register(api)
	}
	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	dm := service.NewDependencyManager("public", cfg)
	aplog.SetDefaultLog(dm.GetRootLogger())
	logger := dm.GetLogger()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

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
			logger.Info("running healt checker", "addr", healthChecker.Addr)
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
