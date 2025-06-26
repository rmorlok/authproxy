package admin_api

import (
	"context"
	"fmt"
	"github.com/gin-gonic/contrib/cors"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/service/public/routes"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

func GetCorsConfig(cfg config.C) *cors.Config {
	root := cfg.GetRoot()
	marketplaceUrl := root.Marketplace.GetBaseUrl()

	return root.Public.CorsVal.ToGinCorsConfig(&cors.Config{
		AllowedOrigins:   []string{marketplaceUrl},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "HEAD"},
		AllowedHeaders:   []string{"Origin", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Cache-Control", "Content-Language", "Content-Length", "Content-Type", "Expires", "Last-Modified", "Pragma"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func GetGinServer(
	cfg config.C,
	db database.DB,
	redis redis.R,
	c connectors.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) (server *gin.Engine, healthChecker *gin.Engine) {
	root := cfg.GetRoot()
	authService := auth.NewService(cfg, &root.Public, db, redis, logger)

	server = api_common.GinForService(&root.Public)

	if root.Public.Port() != root.Public.HealthCheckPort() {
		healthChecker = api_common.GinForService(&root.Public)
	} else {
		healthChecker = server
	}

	corsConfig := GetCorsConfig(cfg)
	if corsConfig != nil {
		server.Use(cors.New(*corsConfig))
	}

	if root.Public.StaticVal != nil {
		// Static content
		server.Use(static.Serve(root.Public.StaticVal.MountAtPath, static.LocalFile(root.Public.StaticVal.ServeFromPath, true)))
	}

	healthChecker.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "public",
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

	routesOauth2 := routes.NewOauth2Routes(cfg, authService, db, redis, c, httpf, encrypt, logger)
	routesSession := routes.NewSessionRoutes(cfg, authService, db, redis, httpf, encrypt, logger)
	routesOauth2.Register(server)
	routesSession.Register(server)

	return server, healthChecker
}

func Serve(cfg config.C) {
	root := cfg.GetRoot()
	aplog.SetDefaultLog(cfg.GetRootLogger())
	logBuilder := aplog.NewBuilder(cfg.GetRootLogger())
	logBuilder = logBuilder.WithService("public")
	logger := logBuilder.Build()

	if !cfg.IsDebugMode() {
		gin.SetMode(gin.ReleaseMode)
	}

	rs, err := redis.New(context.Background(), cfg, logger)
	if err != nil {
		panic(err)
	}
	defer rs.Close()

	db, err := database.NewConnectionForRoot(root, logger)
	if err != nil {
		panic(err)
	}

	if root.Database.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				database.MigrateMutexKeyName,
				redis.MutexOptionLockFor(root.Database.GetAutoMigrationLockDuration()),
				redis.MutexOptionRetryFor(root.Database.GetAutoMigrationLockDuration()+1*time.Second),
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

	h := httpf.CreateFactory(cfg, rs)
	e := encrypt.NewEncryptService(cfg, db)
	asynqClient := asynq.NewClientFromRedisClient(rs.Client())
	c := connectors.NewConnectorsService(cfg, db, e, asynqClient, logger)

	if root.Connectors.GetAutoMigrate() {
		func() {
			m := rs.NewMutex(
				connectors.MigrateMutexKeyName,
				redis.MutexOptionLockFor(root.Connectors.GetAutoMigrationLockDurationOrDefault()),
				redis.MutexOptionRetryFor(root.Connectors.GetAutoMigrationLockDurationOrDefault()+1*time.Second),
				redis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				redis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := c.MigrateConnectors(context.Background()); err != nil {
				panic(err)
			}
		}()
	}

	server, healthChecker := GetGinServer(cfg, db, rs, c, h, e, logger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		api_common.RunGin(server, fmt.Sprintf(":%d", root.Public.Port()), logger)
	}()

	if server != healthChecker {
		wg.Add(1)
		go func() {
			defer wg.Done()
			api_common.RunGin(healthChecker, fmt.Sprintf(":%d", root.Public.HealthCheckPort()), logger)
		}()
	}

	wg.Wait()
	logger.Info("Public shutting down")
	defer logger.Info("Public shutdown complete")
}
