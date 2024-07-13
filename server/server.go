package server

import (
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func rateKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func rateErrorHandler(c *gin.Context, info ratelimit.Info) {
	c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
}

func GetCorsConfig() cors.Config {
	config := cors.DefaultConfig()

	config.AllowOrigins = GetEnvironmentVariables().CorsAllowedOrigins
	config.AllowCredentials = true

	return config
}

func GetGinServer() *gin.Engine {
	authService := GetAuthService()
	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 3,
	})

	router := gin.Default()

	router.Use(authService.Optional())
	router.Use(cors.New(GetCorsConfig()))

	// Static content
	router.Use(static.Serve("/", static.LocalFile("./client/build", true)))

	// setup auth routes
	router.GET("/login", authService.LoginHandler())
	router.GET("/auth/*auth", authService.AuthHandler())
	router.GET("/avatar", authService.Optional(), authService.AvatarHandler())

	api := router.Group("/api", authService.Required())
	{
		mw := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
			ErrorHandler: rateErrorHandler,
			KeyFunc:      rateKeyFunc,
		})

		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})
		api.GET("/me", mw, GetMe)
		api.GET("/albums", mw, GetAlbums)
		api.GET("/albums/:id", mw, GetAlbumByID)
		api.POST("/albums", mw, PostAlbums)
	}

	return router
}

func Serve() {
	r := GetGinServer()
	r.Run("localhost:8080")
}
