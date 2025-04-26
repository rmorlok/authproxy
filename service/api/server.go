package admin_api

import (
	"context"
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/service/api/routes"
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

func GetGinServer(
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) (server *gin.Engine, healthChecker *gin.Engine) {
	authService := auth.NewService(cfg, &cfg.GetRoot().Api, db, redis, logger)

	rlstore := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  1 * time.Minute,
		Limit: 10_000,
	})

	server = api_common.GinForService(&cfg.GetRoot().Api)

	if cfg.GetRoot().Public.Port() != cfg.GetRoot().Public.HealthCheckPort() {
		healthChecker = api_common.GinForService(&cfg.GetRoot().Api)
	} else {
		healthChecker = server
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "api",
			"message": "pong",
		})
	})

	healthChecker.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
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

		c.PureJSON(status, gin.H{
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
	routesConnections := routes.NewConnectionsRoutes(cfg, authService, db, redis, httpf, encrypt, logger)

	api := server.Group("/api", rl)

	routesConnectors.Register(api)
	routesConnections.Register(api)

	return server, healthChecker
}

func Serve(cfg config.C) {
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("api")
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

	httpf := httpf.CreateFactory(cfg, rs)
	encrypt := encrypt.NewEncryptService(cfg, db)

	server, healthChecker := GetGinServer(cfg, db, rs, httpf, encrypt, logger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Run(fmt.Sprintf(":%d", cfg.GetRoot().Api.Port()))
	}()

	if server != healthChecker {
		wg.Add(1)
		go func() {
			defer wg.Done()
			healthChecker.Run(fmt.Sprintf(":%d", cfg.GetRoot().Api.HealthCheckPort()))
		}()
	}

	wg.Wait()
}
