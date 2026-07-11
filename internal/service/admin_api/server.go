package admin_api

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	common_routes "github.com/rmorlok/authproxy/internal/routes"
	"github.com/rmorlok/authproxy/internal/service"
	admin_api_swagger "github.com/rmorlok/authproxy/internal/service/admin_api/swagger"
	adminembed "github.com/rmorlok/authproxy/ui/admin/embed"
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
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"},
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

	server := apgin.ForService(service, logger, dm.GetConfig().IsDebugMode(),
		apgin.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry, dm.GetServiceId()))

	corsConfig := GetCorsConfig(dm.GetConfig())
	if corsConfig != nil {
		server.Use(apgin.NewCorsMiddleware(*corsConfig, logger))
	}

	if service.StaticVal != nil {
		// Static content: prefer the compiled-in admin UI; fall back to an
		// on-disk directory when ServeFromPath is set (local iteration,
		// custom builds).
		if service.StaticVal.IsEmbedded() {
			server.Use(apgin.ServeEmbeddedStatic(service.StaticVal.MountAtPath, adminembed.FS()))
		} else {
			sfs := static.LocalFile(service.StaticVal.ServeFromPath, true)
			server.Use(static.Serve(service.StaticVal.MountAtPath, sfs))
		}
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
		healthChecker = apgin.ForService(service, logger, dm.GetConfig().IsDebugMode(),
			apgin.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry, dm.GetServiceId()))
	} else {
		healthChecker = server
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "admin-api",
			"message": "pong",
		})
	})

	dm.RegisterDatabasePing()
	dm.RegisterRedisPing()
	dm.RegisterAppMetricsPing()

	healthChecker.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		results, allOk := dm.RunPings(ctx)
		status := http.StatusOK
		if !allOk {
			status = http.StatusServiceUnavailable
		}

		response := gin.H{"service": "admin-api", "ok": allOk}
		for k, v := range results {
			response[k] = v
		}
		c.PureJSON(status, response)
	})

	routesConnectors := common_routes.NewConnectorsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetCoreService(),
		dm.GetEncryptService(),
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
	routesRequestEvents := common_routes.NewRequestEventsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetAppMetricsService(),
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
	routesKeys := common_routes.NewKeysRoutes(
		dm.GetConfig(),
		authService,
		dm.GetCoreService(),
	)
	routesRateLimits := common_routes.NewRateLimitsRoutes(
		dm.GetConfig(),
		authService,
		dm.GetCoreService(),
	)
	routesTaskMonitoring := common_routes.NewTaskMonitoringRoutes(
		dm.GetConfig(),
		authService,
		dm.GetAsyncInspector(),
		dm.GetEncryptService(),
	)
	workflowDiagnostics, err := dm.GetWorkflowRuntime().DiagnosticBackend()
	if err != nil {
		return nil, nil, err
	}
	routesWorkflowMonitoring := common_routes.NewWorkflowMonitoringRoutes(
		authService,
		workflowDiagnostics,
		dm.GetEncryptService(),
	)
	routesNotifications := common_routes.NewNotificationsRoutes(
		authService,
		dm.GetDatabase(),
	)

	api := server.Group("/api/v1")

	routesConnectors.Register(api)
	routesConnections.Register(api)
	routesNamespaces.Register(api)
	routesKeys.Register(api)
	routesRateLimits.Register(api)
	routesRequestEvents.Register(api)
	routesActors.Register(api)
	routesTaskMonitoring.Register(api)
	routesWorkflowMonitoring.Register(api)
	routesNotifications.Register(api)

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

	if service.StaticVal != nil {
		// SPA fallback: deep links like /connectors must serve the admin SPA's
		// index.html so React Router can resolve them client-side.
		mountPrefix := strings.TrimSuffix(service.StaticVal.MountAtPath, "/")
		var serveIndex func(c *gin.Context)
		if service.StaticVal.IsEmbedded() {
			indexHTML, readErr := fs.ReadFile(adminembed.FS(), "index.html")
			serveIndex = func(c *gin.Context) {
				if readErr != nil {
					c.Status(http.StatusNotFound)
					return
				}
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			}
		} else {
			indexPath := filepath.Join(service.StaticVal.ServeFromPath, "index.html")
			serveIndex = func(c *gin.Context) {
				c.File(indexPath)
			}
		}
		server.NoRoute(func(c *gin.Context) {
			if c.Request.Method != http.MethodGet {
				return
			}
			p := c.Request.URL.Path
			// Don't shadow API or swagger — they should keep returning their own 404s.
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/swagger") {
				return
			}
			if mountPrefix != "" && !strings.HasPrefix(p, mountPrefix) {
				return
			}
			serveIndex(c)
		})
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

	// Initialise telemetry early; flush last so other shutdowns can still emit.
	dm.GetTelemetry()
	defer dm.ShutdownTelemetry()

	defer dm.GetRedisClient().Close()

	dm.AutoMigrateAll()

	defer dm.ShutdownDatabase()
	defer dm.ShutdownWorkflowRuntime()
	defer dm.GetEncryptService().Shutdown()

	server, healthChecker, err := GetGinServer(dm)
	if err != nil {
		panic(err)
	}

	// Boot the rate-limit cache refresher before serving traffic so the
	// initial snapshot lands before the proxy starts evaluating rules.
	stopRateLimitRefresher := dm.StartRateLimitRefresher(context.Background())
	defer stopRateLimitRefresher()

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

	logger.Info("Admin API shutting down")
	defer logger.Info("Admin API shutdown complete")
}
