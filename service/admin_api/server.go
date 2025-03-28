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
	"github.com/rmorlok/authproxy/redis"
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

func GetGinServer(cfg config.C, db database.DB, redis redis.R) *gin.Engine {
	authService := auth.NewService(cfg, &cfg.GetRoot().AdminApi, db, redis)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 3,
	})

	router := api_common.GinForService(&cfg.GetRoot().AdminApi)

	//router.Use(authService.Optional())
	//router.Use(cors.New(GetCorsConfig()))

	// Static content
	router.Use(static.Serve("/", static.LocalFile("./client/build", true)))

	router.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "admin-api",
			"message": "pong",
		})
	})

	router.GET("/healthz", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "admin-api",
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

	rs, err := redis.New(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	db, err := database.NewConnectionForRoot(cfg.GetRoot())
	if err != nil {
		panic(err)
	}

	if cfg.GetRoot().Database.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				database.MigrateMutexKeyName,
				redis.MutexOptionLockFor(cfg.GetRoot().Database.GetAutoMigrationLockDuration()),
				redis.MutexOptionRetryFor(cfg.GetRoot().Database.GetAutoMigrationLockDuration()+1*time.Second),
				redis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				redis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := db.Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}

	r := GetGinServer(cfg, db, rs)
	r.Run(fmt.Sprintf(":%d", cfg.GetRoot().AdminApi.Port()))
}
