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
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/rmorlok/authproxy/internal/workflows"
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
	logRetriever      app_metrics.LogRetriever
	appMetricsService *app_metrics.StorageService
	e                 encrypt.E
	asynqClient       apasynq.Client
	asynqInspector    *asynq.Inspector
	workflowRuntime   *workflows.Runtime
	c                 coreIface.C
	pings             map[string]PingFunc

	telemetry     *aptelemetry.Providers
	telemetryOnce sync.Once
	telemetryErr  error

	rootLogger     *slog.Logger
	rootLoggerOnce sync.Once

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

// RegisterAppMetricsPing registers a ping for the app metrics service.
func (dm *DependencyManager) RegisterAppMetricsPing() {
	dm.RegisterPing("appMetrics", func(ctx context.Context) bool {
		return dm.GetAppMetricsService().Ping(ctx)
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
		dm.logBuilder = aplog.NewBuilder(dm.GetRootLogger())
	}

	return dm.logBuilder
}

// GetRootLogger returns the application-wide root slog.Logger, wrapped with
// the telemetry-aware handler from internal/aplog so every emitted record
// gains trace_id / span_id when in a traced context (and is fanned to the
// OTel logs pipeline when telemetry.signals.logs is on). Cached on first
// call — every other DM lookup that needs a logger derives from this one,
// so the wrap happens exactly once per process.
//
// Force-initialises telemetry providers before wrapping so the OTel logs
// bridge picks up the live LoggerProvider regardless of call order in the
// service's Serve func.
func (dm *DependencyManager) GetRootLogger() *slog.Logger {
	dm.rootLoggerOnce.Do(func() {
		providers := dm.GetTelemetry()
		dm.rootLogger = aplog.WrapWithTelemetry(
			dm.GetConfigRoot().GetRootLogger(),
			providers,
			dm.GetConfigRoot().Telemetry,
		)
	})
	return dm.rootLogger
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
		dm.r, err = apredis.NewForRoot(
			context.Background(),
			dm.GetConfig().GetRoot(),
			apredis.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry),
		)
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

			if err := workflows.Migrate(
				dm.GetConfigRoot(),
				dm.GetLogBuilder().WithComponent("workflows").Build(),
			); err != nil {
				panic(fmt.Errorf("failed to migrate workflow database: %w", err))
			}
		}()
	}
}

func (dm *DependencyManager) GetAppMetricsService() *app_metrics.StorageService {
	ctx := context.Background()
	var err error
	if dm.appMetricsService == nil {
		dm.appMetricsService, err = app_metrics.NewStorageService(
			ctx,
			dm.GetConfigRoot().AppMetrics,
			pagination.NewRandomCursorEncryptor(),
			dm.GetEncryptService(),
			dm.GetLogger(),
			database.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry),
		)

		if err != nil {
			panic(err)
		}
	}

	return dm.appMetricsService
}

func (dm *DependencyManager) GetRateLimitFactory() *ratelimit.Factory {
	store := ratelimit.NewStore(dm.GetRedisClient())
	return ratelimit.NewFactory(store, dm.GetLogger())
}

// GetRateLimitEnforcerFactory returns the middleware factory that
// evaluates proxy-side RateLimit resources against in-flight requests
// (#223). Reads from the same in-memory cache that the Refresher (#219)
// populates. Construction is cheap; the heavy lifting happens per-request
// inside the round-tripper.
func (dm *DependencyManager) GetRateLimitEnforcerFactory() *ratelimit.EnforcerFactory {
	return ratelimit.NewEnforcerFactory(
		dm.GetRateLimitCache(),
		dm.GetRedisClient(),
		dm.GetLogBuilder().WithComponent("ratelimit-enforcer").Build(),
	)
}

func (dm *DependencyManager) GetHttpf() httpf.F {
	if dm.httpf == nil {
		// Ordering matters: each entry wraps the previous, so the *last*
		// entry in this slice becomes the outermost in execution order.
		// CreateFactory itself appends the requestLog factory last so it
		// surrounds everything and synthetic 429s still produce log
		// entries. Within this slice:
		//   - reactive 429 limiter is innermost so it can short-circuit
		//     a request that's already in cool-down before any other
		//     middleware does work
		//   - the proxy-side rate-limit enforcer (#223) runs immediately
		//     outside the reactive limiter so a rule rejection
		//     short-circuits the reactive check too — but its work is
		//     still covered by the telemetry span that wraps it
		//   - telemetry wraps both rate-limit middlewares so the client
		//     span covers any retries / rate-limit waits they emit
		// NewTelemetryFactory returns (nil, nil) when telemetry is
		// disabled, in which case telemetry simply drops out of the chain.
		middlewares := []httpf.RoundTripperFactory{
			dm.GetRateLimitFactory(),
			dm.GetRateLimitEnforcerFactory(),
		}
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
			dm.GetAppMetricsService(),
			dm.GetLogger(),
			middlewares...,
		)
	}

	return dm.httpf
}

func (dm *DependencyManager) AutoMigrateAppMetricsService() {
	if dm.GetConfigRoot().AppMetrics.GetAutoMigrate() {
		store := dm.GetAppMetricsService()
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

func (dm *DependencyManager) GetWorkflowRuntime() *workflows.Runtime {
	if dm.workflowRuntime == nil {
		r, err := workflows.NewRuntime(
			dm.GetConfigRoot(),
			dm.GetTelemetry(),
			dm.GetLogBuilder().WithComponent("workflows").Build(),
		)
		if err != nil {
			panic(fmt.Errorf("failed to construct workflow runtime: %w", err))
		}

		dm.workflowRuntime = r
	}

	return dm.workflowRuntime
}

func (dm *DependencyManager) RegisterWorkflowRuntimePing() {
	dm.RegisterPing("workflowRuntime", func(ctx context.Context) bool {
		return dm.GetWorkflowRuntime().Ping(ctx)
	})
}

func (dm *DependencyManager) ShutdownWorkflowRuntime() {
	if dm.workflowRuntime == nil {
		return
	}

	if err := dm.workflowRuntime.Close(); err != nil {
		dm.GetLogger().Warn("failed to close workflow runtime", "error", err)
	}
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
			core.WithRateLimitCache(dm.GetRateLimitCache()),
			core.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry),
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
	dm.AutoMigrateAppMetricsService()
	dm.AutoMigrateSyncKeysToDatabase()
	dm.AutoMigrateCore()
	dm.AutoMigratePredefinedActors()
}
