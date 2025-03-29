package worker

import (
	context2 "context"
	"fmt"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"log"
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

	httpf := httpf.CreateFactory(cfg, rs)
	encrypt := encrypt.NewEncryptService(cfg, db)

	workerConfig := cfg.GetRoot().Worker
	router := api_common.GinForService(&workerConfig)

	router.GET("/ping", func(c *gin.Context) {
		c.PureJSON(http.StatusOK, gin.H{
			"service": "worker",
			"message": "pong",
		})
	})

	asyncHasError := false
	asyncRunning := false
	asyncHealthChecker := func(err error) {
		asyncHasError = err != nil
	}

	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.AsContext(c.Request.Context()), 1*time.Second)
		defer cancel()

		dbChan := make(chan bool, 1)
		redisChan := make(chan bool, 1)

		go func() {
			dbChan <- db.Ping(ctx)
		}()

		go func() {
			redisChan <- rs.Ping(ctx)
		}()

		dbOk := <-dbChan
		redisOk := <-redisChan
		everythingOk := dbOk && redisOk
		status := http.StatusOK
		if !everythingOk {
			status = http.StatusServiceUnavailable
		}

		c.PureJSON(status, gin.H{
			"service": "worker",
			"db":      dbOk,
			"redis":   redisOk,
			"asynq":   asyncRunning && !asyncHasError,
			"ok":      everythingOk,
		})
	})

	srv := asynq.NewServerFromRedisClient(
		rs.Client(),
		asynq.Config{
			HealthCheckFunc: asyncHealthChecker,
			Concurrency:     workerConfig.GetConcurrency(context.Background()),
			BaseContext: func() context2.Context {
				return context.Background()
			},
		},
	)

	mux := asynq.NewServeMux()

	oauth2.
		NewTaskHandler(cfg, db, rs, httpf, encrypt).
		RegisterTasks(mux)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		if err := srv.Run(mux); err != nil {
			asyncHasError = true
			log.Fatalf("could not run async server: %v", err)
		}
		asyncRunning = false
		wg.Done()
	}()
	
	wg.Add(1)
	go func() {
		if err := router.Run(fmt.Sprintf(":%d", cfg.GetRoot().Public.Port())); err != nil {
			log.Fatalf("could not run gin server: %v", err)
		}
	}()

	wg.Wait()
}
