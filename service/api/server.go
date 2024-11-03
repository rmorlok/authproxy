package admin_api

import (
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/service/api/routes"
	"net/http"
	"time"
)

func rateKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func rateErrorHandler(c *gin.Context, info ratelimit.Info) {
	c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
}

func GetGinServer(cfg config.C) *gin.Engine {
	authService := auth.StandardAuthService(cfg, config.ServiceIdApi)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 3,
	})

	router := api_common.GinForService("api", &cfg.GetRoot().AdminApi)

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "api",
			"message": "pong",
		})
	})

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "api",
			"ok":      true,
		})
	})

	rl := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
		ErrorHandler: rateErrorHandler,
		KeyFunc:      rateKeyFunc,
	})

	routesConnectors := routes.NewConnectorsRoutes(cfg, authService)

	api := router.Group("/api", rl)
	routesConnectors.Register(api)

	return router
}

func Serve(cfg config.C) {
	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := GetGinServer(cfg)
	r.Run(fmt.Sprintf(":%d", cfg.GetRoot().Api.Port))
}
