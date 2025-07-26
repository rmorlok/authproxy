package redis

import (
	"context"
	"github.com/alicebob/miniredis"
	"github.com/bsm/redislock"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/config"
	"log"
	"log/slog"
	"sync"
)

var miniredisServer *miniredis.Miniredis
var miniredisClient *redis.Client
var miniredisMutex sync.Mutex
var miniredisErr error

var redisClient *redis.Client
var redisOnce sync.Once
var redisErr error

type wrapper struct {
	client     *redis.Client
	secretKey  config.KeyData
	lockClient *redislock.Client
	logger     *slog.Logger
}

// New creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration.
func New(ctx context.Context, c config.C, logger *slog.Logger) (R, error) {
	return NewForRoot(ctx, c.GetRoot(), logger)
}

// NewForRoot creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration. Same as NewConnection.
func NewForRoot(ctx context.Context, root *config.Root, logger *slog.Logger) (R, error) {
	redisConfig := root.Redis
	secretKey := root.SystemAuth.GlobalAESKey

	switch v := redisConfig.(type) {
	case *config.RedisMiniredis:
		return NewMiniredis(v, secretKey, logger)
	case *config.RedisReal:
		return NewRedis(ctx, v, secretKey, logger)
	default:
		return nil, errors.New("redis type not supported")
	}
}

// NewMiniredis creates a new database connection to a SQLite database.
//
// Parameters:
// - redisConfig: the configuration for the SQLite database
// - secretKey: the AES key used to secure cursors
func NewMiniredis(redisConfig *config.RedisMiniredis, secretKey config.KeyData, logger *slog.Logger) (R, error) {
	if miniredisServer == nil {
		miniredisMutex.Lock()
		defer miniredisMutex.Unlock()

		// Check again now that we are the primary
		if miniredisServer == nil {
			var err error
			// Create a new instance of miniredis for testing purposes
			miniredisServer, err = miniredis.Run()
			if err != nil {
				miniredisErr = errors.Wrap(err, "failed to start miniredis server")
			}

			// Configure the Redis client to use the miniredis instance
			miniredisClient = redis.NewClient(&redis.Options{
				Addr: miniredisServer.Addr(),
			})

			// Test the connection to ensure it's working
			_, err = miniredisClient.Ping(context.Background()).Result()
			if err != nil {
				miniredisServer.Close()
				miniredisErr = errors.Wrap(err, "failed to connect to miniredis client")
			}
		}
	}

	if miniredisErr != nil {
		return nil, miniredisErr
	}

	// Return the wrapper containing the Redis client
	return &wrapper{
		client:     miniredisClient,
		secretKey:  secretKey,
		lockClient: redislock.New(miniredisClient),
		logger:     logger,
	}, nil
}

// NewRedis creates a new redis connection to a real redis instance.
//
// Parameters:
// - redisConfig: the configuration for the Redis instance
// - secretKey: the AES key used to secure cursors
func NewRedis(ctx context.Context, redisConfig *config.RedisReal, secretKey config.KeyData, logger *slog.Logger) (R, error) {
	if redisClient == nil {
		redisOnce.Do(func() {
			var err error

			cfg, err := redisConfig.ToRedisOptions(ctx)
			if err != nil {
				redisErr = errors.Wrap(err, "failed to convert redis config to redis options")
				return
			}

			// Configure the Redis client with the provided configuration
			redisClient = redis.NewClient(cfg)

			// Test the connection to ensure it's working
			_, err = redisClient.Ping(context.Background()).Result()
			if err != nil {
				redisErr = errors.Wrap(err, "failed to connect to real Redis server")
				return
			}
		})
	}

	if redisErr != nil {
		return nil, redisErr
	}

	return &wrapper{
		client:     redisClient,
		secretKey:  secretKey,
		lockClient: redislock.New(redisClient),
		logger:     logger,
	}, nil
}

func (w *wrapper) Ping(ctx context.Context) bool {
	if w.client == nil {
		log.Println("redis client is unexpectedly nil ")
		return false
	}

	_, err := w.client.Ping(ctx).Result()
	if err != nil {
		log.Println(errors.Wrap(err, "failed to connect to redis server"))
		return false
	}

	return true
}

func (w *wrapper) Close() error {
	return w.client.Close()
}

func (w *wrapper) Client() *redis.Client {
	return w.client
}

func MustApplyTestConfig(cfg config.C) (config.C, R) {
	// Avoid shared singletons for test cases, while still going through wireup logic
	miniredisServerPrevious := miniredisServer
	miniredisClientPrevious := miniredisClient
	miniredisErrPrevious := miniredisErr
	defer func() {
		miniredisServer = miniredisServerPrevious
		miniredisClient = miniredisClientPrevious
		miniredisErr = miniredisErrPrevious
	}()
	miniredisServer = nil
	miniredisClient = nil
	miniredisErr = nil

	if cfg == nil {
		cfg = config.FromRoot(&config.Root{})
	}

	root := cfg.GetRoot()

	if root == nil {
		panic("No root in config")
	}

	redisCfg := &config.RedisMiniredis{
		Provider: config.RedisProviderMiniredis,
	}
	root.Redis = redisCfg
	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &config.KeyDataRandomBytes{}
	}

	r, err := NewMiniredis(redisCfg, root.SystemAuth.GlobalAESKey, cfg.GetRootLogger())
	if err != nil {
		panic(err)
	}

	return cfg, r
}
