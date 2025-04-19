package admin_api

import (
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

func rateKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func rateErrorHandler(c *gin.Context, info ratelimit.Info) {
	c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
}

func GetGinServer(cfg config.C, db database.DB, redis redis.R, logger *slog.Logger) (server *gin.Engine, healthChecker *gin.Engine) {
	authService := auth.NewService(cfg, &cfg.GetRoot().AdminApi, db, redis, logger)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 3,
	})

	server = api_common.GinForService(&cfg.GetRoot().AdminApi)

	if cfg.GetRoot().Public.Port() != cfg.GetRoot().Public.HealthCheckPort() {
		healthChecker = api_common.GinForService(&cfg.GetRoot().AdminApi)
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

	api := server.Group("/api", authService.AdminOnly())
	{
		mw := ratelimit.RateLimiter(rlstore, &ratelimit.Options{
			ErrorHandler: rateErrorHandler,
			KeyFunc:      rateKeyFunc,
		})

		api.GET("/domains", mw, ListDomains)
	}

	return server, healthChecker
}

func Serve(cfg config.C) {
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("admin-api")
	logger := logBuilder.Build()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	rs, err := redis.New(context.Background(), cfg, logger)
	if err != nil {
		panic(err)
	}

	db, err := database.NewConnectionForRoot(cfg.GetRoot(), logger)
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

	server, healthChecker := GetGinServer(cfg, db, rs, logger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Run(fmt.Sprintf(":%d", cfg.GetRoot().AdminApi.Port()))
	}()

	if server != healthChecker {
		wg.Add(1)
		go func() {
			defer wg.Done()
			healthChecker.Run(fmt.Sprintf(":%d", cfg.GetRoot().AdminApi.HealthCheckPort()))
		}()
	}

	wg.Wait()

}
