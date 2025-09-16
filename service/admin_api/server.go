package admin_api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	common_routes "github.com/rmorlok/authproxy/routes"
	"github.com/rmorlok/authproxy/service"
)

func GetCorsConfig(cfg config.C) *cors.Config {
	root := cfg.GetRoot()
	admin := root.AdminApi

	var baseConfig *cors.Config
	uiBaseUrl := admin.UiBaseUrl()
	if uiBaseUrl != "" {
		// If adm in ui is configured as an external service, allow CORS to that host
		baseConfig = &cors.Config{
			AllowOrigins:     []string{uiBaseUrl},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "HEAD"},
			AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Cookie", "X-Xsrf-Token"},
			ExposeHeaders:    []string{"Cache-Control", "Content-Language", "Content-Length", "Content-Type", "Expires", "Last-Modified", "Pragma", "X-Xsrf-Token"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}
	}

	return admin.CorsVal.ToGinCorsConfig(baseConfig)
}

func GetGinServer(
	dm *service.DependencyManager,
) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	root := dm.GetConfigRoot()
	service := &root.AdminApi
	logger := dm.GetLogger()
	authService := auth.NewService(
		dm.GetConfig(),
		service,
		dm.GetDatabase(),
		dm.GetRedisWrapper(),
		dm.GetEncryptService(),
		logger,
	)

	server := api_common.GinForService(service)

	corsConfig := GetCorsConfig(dm.GetConfig())
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

	routesRequestLog := common_routes.NewRequestLogRoutes(
		dm.GetConfig(),
		authService,
		dm.GetRequestLogRetriever(),
	)

	// api := server.Group("/api/v1", authService.AdminOnly())
	api := server.Group("/api/v1")
	routesRequestLog.Register(api)

	if service.SupportsSession() && service.SupportsUi() {
		routesSession := common_routes.NewSessionRoutes(
			dm.GetConfig(),
			authService,
			dm.GetDatabase(),
			dm.GetRedisWrapper(),
			dm.GetHttpf(),
			dm.GetEncryptService(),
			logger,
		)
		routesSession.Register(api)
	}

	if service.SupportsUi() {
		routesError := common_routes.NewErrorRoutes(dm.GetConfig())
		routesError.Register(server)
	}

	return service.GetServerAndHealthChecker(server, healthChecker)
}

func Serve(cfg config.C) {
	dm := service.NewDependencyManager("admin-api", cfg)
	aplog.SetDefaultLog(dm.GetRootLogger())
	logger := dm.GetLogger()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	defer dm.GetRedisWrapper().Close()

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

	logger.Info("Admin API shutting down")
	defer logger.Info("Admin API shutdown complete")
}
