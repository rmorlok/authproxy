package admin_api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	"github.com/rmorlok/authproxy/internal/service"
	admin_api_swagger "github.com/rmorlok/authproxy/internal/service/admin_api/swagger"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
		dm.GetRedisClient(),
		dm.GetEncryptService(),
		logger,
	)

	server := api_common.GinForService(service, logger, dm.GetConfig().IsDebugMode())

	corsConfig := GetCorsConfig(dm.GetConfig())
	if corsConfig != nil {
		logger.Info("Enabling CORS")
		server.Use(cors.New(*corsConfig))
	}

	// Swagger documentation endpoint
	swaggerHost := service.GetBaseUrl()
	swaggerHost = strings.TrimPrefix(swaggerHost, "https://")
	swaggerHost = strings.TrimPrefix(swaggerHost, "http://")
	admin_api_swagger.SwaggerInfoadmin_api.Host = swaggerHost
	admin_api_swagger.SwaggerInfoadmin_api.InfoInstanceName = "admin_api"
	server.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	server.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.InstanceName("admin_api"),
	))

	var healthChecker *gin.Engine
	if service.Port() != service.HealthCheckPort() {
		healthChecker = api_common.GinForService(service, logger, dm.GetConfig().IsDebugMode())
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
	routesTaskMonitoring := common_routes.NewTaskMonitoringRoutes(
		dm.GetConfig(),
		authService,
		dm.GetAsyncInspector(),
	)

	api := server.Group("/api/v1")

	routesConnectors.Register(api)
	routesConnections.Register(api)
	routesNamespaces.Register(api)
	routesRequestLog.Register(api)
	routesActors.Register(api)
	routesTaskMonitoring.Register(api)

	if service.SupportsSession() && service.SupportsUi() {
		routesSession := common_routes.NewSessionRoutes(
			dm.GetConfig(),
			service.Ui,
			authService,
			dm.GetDatabase(),
			dm.GetRedisClient(),
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
			logger.Error(err.Error(), "error", err)
		}
	}()

	if healthChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("running health checker", "addr", healthChecker.Addr)
			err := api_common.RunServer(healthChecker, logger)
			if err != nil {
				logger.Error(err.Error(), "error", err)
			}
		}()
	}

	wg.Wait()

	logger.Info("Admin API shutting down")
	defer logger.Info("Admin API shutdown complete")
}
