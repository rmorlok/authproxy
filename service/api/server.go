package admin_api

import (
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"net/http"
	"time"
)

func rateKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func rateErrorHandler(c *gin.Context, info ratelimit.Info) {
	c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
}

//func GetCorsConfig() cors.Config {
//	config := cors.DefaultConfig()
//
//	config.AllowOrigins = GetEnvironmentVariables().CorsAllowedOrigins
//	config.AllowCredentials = true
//
//	return config
//}

func GetGinServer(cfg config.C) *gin.Engine {
	authService := auth.StandardAuthService(cfg, config.ServiceIdAdminApi)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 3,
	})

	router := api_common.GinForService("api", &cfg.GetRoot().AdminApi)

	//router.Use(authService.Optional())
	//router.Use(cors.New(GetCorsConfig()))

	// Static content
	router.Use(static.Serve("/", static.LocalFile("./client/build", true)))

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

	api := router.Group("/api", authService.AdminOnly())
	{
		mw := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
			ErrorHandler: rateErrorHandler,
			KeyFunc:      rateKeyFunc,
		})

		api.GET("/domains", mw, ListDomains)
	}

	return router
}

func Serve(cfg config.C) {
	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := GetGinServer(cfg)
	r.Run(fmt.Sprintf(":%d", cfg.GetRoot().Api.Port))
}
