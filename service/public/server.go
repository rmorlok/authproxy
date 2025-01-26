package admin_api

import (
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/service/public/routes"
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

func GetGinServer(
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
) *gin.Engine {
	authService := auth.StandardAuthService(cfg, &cfg.GetRoot().Public, db, redis)

	router := api_common.GinForService(&cfg.GetRoot().Public)

	//router.Use(authService.Optional())
	//router.Use(cors.New(GetCorsConfig()))

	// Static content
	router.Use(static.Serve("/", static.LocalFile("./client/build", true)))

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "auth",
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

	routesOauth2 := routes.NewOauth2Routes(cfg, authService, db, redis, httpf, encrypt)
	routesOauth2.Register(router)

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

	httpf := httpf.CreateFactory(cfg, redis)
	encrypt := encrypt.NewEncryptService(cfg, db)

	r := GetGinServer(cfg, db, redis, httpf, encrypt)
	r.Run(fmt.Sprintf(":%d", cfg.GetRoot().Public.Port()))
}
