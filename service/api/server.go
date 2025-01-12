package admin_api

import (
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/redis"
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

func GetGinServer(cfg config.C, db database.DB, redis *redis.Wrapper) *gin.Engine {
	authService := auth.StandardAuthService(cfg, config.ServiceIdApi, db, redis)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 10_000,
	})

	router := api_common.GinForService("api", &cfg.GetRoot().AdminApi)

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "api",
			"message": "pong",
		})
	})

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.AsContext(c.Request.Context()), 1*time.Second)
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

		c.JSON(status, gin.H{
			"service": "api",
			"db":      dbOk,
			"redis":   redisOk,
			"ok":      everythingOk,
		})
	})

	rl := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
		ErrorHandler: rateErrorHandler,
		KeyFunc:      rateKeyFunc,
	})

	routesConnectors := routes.NewConnectorsRoutes(cfg, authService)
	routesConnections := routes.NewConnectionsRoutes(cfg, authService, db, redis)

	api := router.Group("/api", rl)

	routesConnectors.Register(api)
	routesConnections.Register(api)

	return router
}

func Serve(cfg config.C) {
	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	redis, err := redis.New(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	db, err := database.NewConnectionForRoot(cfg.GetRoot())
	if err != nil {
		panic(err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		panic(err)
	}

	r := GetGinServer(cfg, db, redis)
	r.Run(fmt.Sprintf(":%d", cfg.GetRoot().Api.Port))
}
