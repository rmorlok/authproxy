package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apauth/tasks"
	authSync "github.com/rmorlok/authproxy/internal/apauth/tasks"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/request_log"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// PingFunc is a function that checks the health of a dependency.
// It returns true if the dependency is healthy.
type PingFunc func(ctx context.Context) bool

type DependencyManager struct {
	serviceId         string
	cfg               config.C
	logBuilder        aplog.Builder
	logger            *slog.Logger
	r                 apredis.Client
	db                database.DB
	httpf             httpf.F
	logRetriever      request_log.LogRetriever
	logStorageService *request_log.StorageService
	e                 encrypt.E
	asynqClient       apasynq.Client
	asynqInspector    *asynq.Inspector
	c                 coreIface.C
	pings             map[string]PingFunc

	telemetry     *aptelemetry.Providers
	telemetryOnce sync.Once
	telemetryErr  error

	// Rate-limit cache + refresher are owned by the dependency manager so
	// the lifecycle is tied to the proxy process. The cache is populated
	// lazily via GetRateLimitCache(); StartRateLimitRefresher() boots the
	// background goroutine and returns a stop function the caller defers.
	rateLimitCache ratelimit.MutableCache
	rateLimitOnce  sync.Once
}

func NewDependencyManager(serviceId string, cfg config.C) *DependencyManager {
	return &DependencyManager{
		serviceId: serviceId,
		cfg:       cfg,
		pings:     make(map[string]PingFunc),
	}
}

// RegisterPing registers a named ping function for health checking.
func (dm *DependencyManager) RegisterPing(name string, fn PingFunc) {
	dm.pings[name] = fn
}

// RunPings runs all registered ping functions concurrently and returns
// a map of results and whether all pings succeeded.
func (dm *DependencyManager) RunPings(ctx context.Context) (map[string]bool, bool) {
	results := make(map[string]bool, len(dm.pings))
	if len(dm.pings) == 0 {
		return results, true
	}

	type pingResult struct {
		name string
		ok   bool
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, fn := range dm.pings {
		wg.Add(1)
		go func(name string, fn PingFunc) {
			defer wg.Done()
			ok := fn(ctx)
			mu.Lock()
			results[name] = ok
			mu.Unlock()
		}(name, fn)
	}

	wg.Wait()

	allOk := true
	for _, ok := range results {
		if !ok {
			allOk = false
			break
		}
	}

	return results, allOk
}

// RegisterDatabasePing registers a ping for the database.
func (dm *DependencyManager) RegisterDatabasePing() {
	dm.RegisterPing("db", func(ctx context.Context) bool {
		return dm.GetDatabase().Ping(ctx)
	})
}

// RegisterRedisPing registers a ping for Redis.
func (dm *DependencyManager) RegisterRedisPing() {
	dm.RegisterPing("redis", func(ctx context.Context) bool {
		return apredis.Ping(ctx, dm.GetRedisClient())
	})
}

// RegisterAsynqClientPing registers a ping for the Asynq client.
func (dm *DependencyManager) RegisterAsynqClientPing() {
	dm.RegisterPing("asynqClient", func(ctx context.Context) bool {
		return dm.GetAsyncClient().Ping() == nil
	})
}

// RegisterLogStoragePing registers a ping for the log storage service.
func (dm *DependencyManager) RegisterLogStoragePing() {
	dm.RegisterPing("logStorage", func(ctx context.Context) bool {
		return dm.GetLogStorageService().Ping(ctx)
	})
}

func (dm *DependencyManager) GetConfig() config.C {
	return dm.cfg
}

func (dm *DependencyManager) GetConfigRoot() *sconfig.Root {
	return dm.cfg.GetRoot()
}

func (dm *DependencyManager) GetServiceId() string {
	return dm.serviceId
}

func (dm *DependencyManager) GetLogBuilder() aplog.Builder {
	if dm.logBuilder == nil {
		dm.logBuilder = aplog.NewBuilder(dm.cfg.GetRootLogger())
	}

	return dm.logBuilder
}

func (dm *DependencyManager) GetRootLogger() *slog.Logger {
	return dm.GetConfigRoot().GetRootLogger()
}

func (dm *DependencyManager) GetLogger() *slog.Logger {
	if dm.logger == nil {
		b := dm.GetLogBuilder()
		b = b.WithService(dm.serviceId)
		dm.logger = b.Build()
	}

	return dm.logger
}

func (dm *DependencyManager) GetRedisClient() apredis.Client {
	if dm.r == nil {
		var err error
		dm.r, err = apredis.NewForRoot(context.Background(), dm.GetConfig().GetRoot())
		if err != nil {
			panic(err)
		}
	}

	return dm.r
}

func (dm *DependencyManager) GetDatabase() database.DB {
	if dm.db == nil {
		var err error
		dm.db, err = database.NewConnectionForRoot(
			dm.GetConfigRoot(),
			dm.GetLogger(),
			database.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry),
		)
		if err != nil {
			panic(err)
		}
	}

	return dm.db
}

// AutoMigrateDatabase will attempt to migrate the database if the root config has auto migrate enabled.
func (dm *DependencyManager) AutoMigrateDatabase() {
	if dm.GetConfigRoot().Database.GetAutoMigrate() {
		func() {
			m := apredis.NewMutex(
				dm.GetRedisClient(),
				database.MigrateMutexKeyName,
				apredis.MutexOptionLockFor(dm.GetConfigRoot().Database.GetAutoMigrationLockDuration()),
				apredis.MutexOptionRetryFor(dm.GetConfigRoot().Database.GetAutoMigrationLockDuration()+1*time.Second),
				apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				apredis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(fmt.Errorf("failed to establish lock for database migration: %w", err))
			}
			defer m.Unlock(context.Background())

			if err := dm.GetDatabase().Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}
}

func (dm *DependencyManager) GetLogStorageService() *request_log.StorageService {
	ctx := context.Background()
	var err error
	if dm.logStorageService == nil {
		dm.logStorageService, err = request_log.NewStorageService(
			ctx,
			dm.GetConfigRoot().HttpLogging,
			pagination.NewRandomCursorEncryptor(),
			dm.GetEncryptService(),
			dm.GetLogger(),
		)

		if err != nil {
			panic(err)
		}
	}

	return dm.logStorageService
}

func (dm *DependencyManager) GetRateLimitFactory() *ratelimit.Factory {
	store := ratelimit.NewStore(dm.GetRedisClient())
	return ratelimit.NewFactory(store, dm.GetLogger())
}

func (dm *DependencyManager) GetHttpf() httpf.F {
	if dm.httpf == nil {
		// Build the additional-middlewares list. Telemetry is appended last
		// so it wraps everything else, making the client span cover any
		// retries / rate-limit waits emitted by the inner middlewares.
		// NewTelemetryFactory returns (nil, nil) when telemetry is disabled,
		// in which case the chain is identical to its pre-telemetry shape.
		middlewares := []httpf.RoundTripperFactory{dm.GetRateLimitFactory()}
		telemetryRT, err := httpf.NewTelemetryFactory(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry)
		if err != nil {
			panic(fmt.Errorf("failed to construct httpf telemetry middleware: %w", err))
		}
		if telemetryRT != nil {
			middlewares = append(middlewares, telemetryRT)
		}

		dm.httpf = httpf.CreateFactory(
			dm.GetConfig(),
			dm.GetRedisClient(),
			dm.GetLogStorageService(),
			dm.GetLogger(),
			middlewares...,
		)
	}

	return dm.httpf
}

func (dm *DependencyManager) AutoMigrateLogStorageService() {
	if dm.GetConfigRoot().HttpLogging.GetAutoMigrate() {
		store := dm.GetLogStorageService()
		err := store.Migrate(context.Background())
		if err != nil {
			panic(err)
		}
	}
}

func (dm *DependencyManager) GetEncryptService() encrypt.E {
	if dm.e == nil {
		dm.e = encrypt.NewEncryptService(dm.GetConfig(), dm.GetDatabase(), dm.GetLogger())
		dm.e.Start()
		dm.GetDatabase().SetCursorEncryptor(dm.e)
	}

	return dm.e
}

func (dm *DependencyManager) GetAsyncDefaultOptions() []asynq.Option {
	root := dm.GetConfigRoot()
	if root == nil || root.Tasks == nil {
		return nil
	}

	opts := []asynq.Option{}

	if root.Tasks.DefaultRetention != nil {
		opts = append(opts, asynq.Retention(root.Tasks.DefaultRetention.Duration))
	}

	return opts
}

func (dm *DependencyManager) GetAsyncClient() apasynq.Client {
	if dm.asynqClient == nil {
		dm.asynqClient = apasynq.WrapClientWithDefaultOptions(
			asynq.NewClientFromRedisClient(dm.GetRedisClient()),
			dm.GetAsyncDefaultOptions(),
		)
	}

	return dm.asynqClient
}

func (dm *DependencyManager) GetAsyncInspector() *asynq.Inspector {
	if dm.asynqInspector == nil {
		dm.asynqInspector = asynq.NewInspectorFromRedisClient(dm.GetRedisClient())
	}

	return dm.asynqInspector
}

// GetTelemetry returns the OTel providers for this service. When telemetry is
// disabled or unconfigured, the returned Providers are no-op implementations.
//
// The first call lazily initialises the SDK; subsequent calls return the same
// Providers. Initialisation failure is panicked, matching the pattern used by
// other dependencies on this manager. Use ShutdownTelemetry to flush and tear
// down before exit.
func (dm *DependencyManager) GetTelemetry() *aptelemetry.Providers {
	dm.telemetryOnce.Do(func() {
		providers, err := aptelemetry.New(
			context.Background(),
			dm.serviceId,
			"",
			dm.GetConfigRoot().Telemetry,
		)
		if err != nil {
			dm.telemetryErr = err
			return
		}
		dm.telemetry = providers
	})

	if dm.telemetryErr != nil {
		panic(fmt.Errorf("failed to initialise telemetry: %w", dm.telemetryErr))
	}

	return dm.telemetry
}

// ShutdownTelemetry flushes and tears down OTel providers if they were
// initialised. Safe to call multiple times. Bounded by aptelemetry.ShutdownTimeout.
func (dm *DependencyManager) ShutdownTelemetry() {
	if dm.telemetry == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), aptelemetry.ShutdownTimeout)
	defer cancel()

	if err := dm.telemetry.Shutdown(ctx); err != nil {
		dm.GetLogger().Warn("telemetry shutdown reported an error", "error", err)
	}
}

func (dm *DependencyManager) GetCoreService() coreIface.C {
	if dm.c == nil {
		dm.c = core.NewCoreService(
			dm.GetConfig(),
			dm.GetDatabase(),
			dm.GetEncryptService(),
			dm.GetRedisClient(),
			dm.GetHttpf(),
			dm.GetAsyncClient(),
			dm.GetLogger(),
		)
	}

	return dm.c
}

// GetRateLimitCache returns the lazily-initialised in-memory rate-limit cache
// for this process. The cache starts empty; call StartRateLimitRefresher()
// to populate and keep it fresh from the database.
func (dm *DependencyManager) GetRateLimitCache() ratelimit.Cache {
	dm.rateLimitOnce.Do(func() {
		dm.rateLimitCache = ratelimit.NewCache()
	})
	return dm.rateLimitCache
}

// StartRateLimitRefresher boots the background goroutine that keeps the
// in-memory rate-limit cache fresh from the database. The returned stop
// function cancels the goroutine and waits for it to exit; api/admin-api
// callers should defer it.
//
// Multiple calls within the same process are safe but only the first
// actually starts a goroutine — subsequent calls return a no-op stop.
func (dm *DependencyManager) StartRateLimitRefresher(ctx context.Context) (stop func()) {
	// Make sure the cache singleton is initialised.
	_ = dm.GetRateLimitCache()
	return ratelimit.StartRefresher(
		ctx,
		dm.GetDatabase(),
		dm.rateLimitCache,
		dm.GetLogBuilder().WithComponent("ratelimit-refresher").Build(),
	)
}

// TODO: this automigrate should not be specific to the connectors config
func (dm *DependencyManager) AutoMigrateCore() {
	if dm.GetConfigRoot().Connectors.GetAutoMigrate() {
		func() {
			m := apredis.NewMutex(
				dm.GetRedisClient(),
				core.MigrateMutexKeyName,
				apredis.MutexOptionLockFor(dm.GetConfigRoot().Connectors.GetAutoMigrationLockDurationOrDefault()),
				apredis.MutexOptionRetryFor(dm.GetConfigRoot().Connectors.GetAutoMigrationLockDurationOrDefault()+1*time.Second),
				apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				apredis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := dm.GetCoreService().Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}
}

// AutoMigratePredefinedActors synchronizes actors from ConfiguredActorsList configuration to the database.
// This only runs for ConfiguredActorsList configuration (not ConfiguredActorsExternalSource which uses cron).
// Uses a distributed Redis lock to ensure only one instance performs the migration.
func (dm *DependencyManager) AutoMigratePredefinedActors() {
	actors := dm.GetConfigRoot().SystemAuth.Actors
	if actors == nil {
		return
	}

	if _, ok := actors.InnerVal.(*sconfig.ConfiguredActorsExternalSource); ok {
		// Don't actually run the sync here, just enqueue a task to run immediately.
		task := authSync.NewSyncActorsExternalSourceTask()
		_, err := dm.GetAsyncClient().Enqueue(task)
		if err != nil {
			panic(fmt.Errorf("failed to enqueue sync actors external source task: %w", err))
		}

		return
	}

	if _, ok := actors.InnerVal.(sconfig.ConfiguredActorsList); !ok {
		// There aren't any other value types that we migrate
		return
	}

	func() {
		m := apredis.NewMutex(
			dm.GetRedisClient(),
			"actor_sync:migrate",
			apredis.MutexOptionLockFor(30*time.Second),
			apredis.MutexOptionRetryFor(31*time.Second),
			apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
			apredis.MutexOptionDetailedLockMetadata(),
		)
		err := m.Lock(context.Background())
		if err != nil {
			panic(fmt.Errorf("failed to establish lock for admin users migration: %w", err))
		}
		defer m.Unlock(context.Background())

		svc := tasks.NewService(
			dm.GetConfig(),
			dm.GetDatabase(),
			dm.GetRedisClient(),
			dm.GetEncryptService(),
			dm.GetLogger(),
		)

		if err := svc.SyncActorList(context.Background()); err != nil {
			panic(fmt.Errorf("failed to sync actors from config list: %w", err))
		}
	}()
}

// AutoMigrateSyncKeysToDatabase syncs encryption key versions from config into the database.
// Uses a Redis sentinel to avoid redundant runs across processes.
func (dm *DependencyManager) AutoMigrateSyncKeysToDatabase() {
	if err := encrypt.SyncKeysToDatabase(
		context.Background(),
		dm.GetConfig(),
		dm.GetDatabase(),
		dm.GetLogger(),
		dm.GetRedisClient(),
	); err != nil {
		panic(fmt.Errorf("failed to sync encryption keys to database: %w", err))
	}
}

func (dm *DependencyManager) AutoMigrateAll() {
	dm.AutoMigrateDatabase()
	dm.AutoMigrateLogStorageService()
	dm.AutoMigrateSyncKeysToDatabase()
	dm.AutoMigrateCore()
	dm.AutoMigratePredefinedActors()
}
