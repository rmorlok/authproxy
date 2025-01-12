package redis

import (
	"github.com/alicebob/miniredis"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"log"
	"sync"
)

var miniredisServer *miniredis.Miniredis
var miniredisClient *redis.Client
var miniredisOnce sync.Once
var miniredisErr error

var redisClient *redis.Client
var redisOnce sync.Once
var redisErr error

type Wrapper struct {
	Client    *redis.Client
	secretKey config.KeyData
}

// New creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration.
func New(ctx context.Context, c config.C) (*Wrapper, error) {
	return NewForRoot(ctx, c.GetRoot())
}

// NewForRoot creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration. Same as NewConnection.
func NewForRoot(ctx context.Context, root *config.Root) (*Wrapper, error) {
	redisConfig := root.Redis
	secretKey := root.SystemAuth.GlobalAESKey

	switch v := redisConfig.(type) {
	case *config.RedisMiniredis:
		return NewMiniredis(v, secretKey)
	case *config.RedisReal:
		return NewRedis(ctx, v, secretKey)
	default:
		return nil, errors.New("redis type not supported")
	}
}

// NewMiniredis creates a new database connection to a SQLite database.
//
// Parameters:
// - redisConfig: the configuration for the SQLite database
// - secretKey: the AES key used to secure cursors
func NewMiniredis(redisConfig *config.RedisMiniredis, secretKey config.KeyData) (*Wrapper, error) {
	if miniredisServer == nil {
		miniredisOnce.Do(func() {
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
		})
	}

	if miniredisErr != nil {
		return nil, miniredisErr
	}

	// Return the wrapper containing the Redis client
	return &Wrapper{
		Client:    miniredisClient,
		secretKey: secretKey,
	}, nil
}

// NewRedis creates a new redis connection to a real redis instance.
//
// Parameters:
// - redisConfig: the configuration for the Redis instance
// - secretKey: the AES key used to secure cursors
func NewRedis(ctx context.Context, redisConfig *config.RedisReal, secretKey config.KeyData) (*Wrapper, error) {
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

	return &Wrapper{
		Client:    redisClient,
		secretKey: secretKey,
	}, nil
}

func (w *Wrapper) Ping(ctx context.Context) bool {
	if w.Client == nil {
		log.Println("redis client is unexpectedly nil ")
		return false
	}

	_, err := w.Client.Ping(ctx).Result()
	if err != nil {
		log.Println(errors.Wrap(err, "failed to connect to redis server"))
		return false
	}

	return true
}

func MustApplyTestConfig(cfg config.C) (config.C, *Wrapper) {
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

	r, err := NewMiniredis(redisCfg, root.SystemAuth.GlobalAESKey)
	if err != nil {
		panic(err)
	}

	return cfg, r
}