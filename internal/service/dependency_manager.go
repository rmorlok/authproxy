package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"

	"github.com/hibiken/asynq"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/rmorlok/authproxy/internal/apasynq"
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
	"github.com/rmorlok/authproxy/internal/sqlh"
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
	sqlDB             *sql.DB
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

	dataEncryptionKeyTelemetry     *encrypt.DataEncryptionKeyTelemetry
	dataEncryptionKeyTelemetryOnce sync.Once
	dataEncryptionKeyTelemetryErr  error

	rootLogger     *slog.Logger
	rootLoggerOnce sync.Once
	rootLoggerErr  error

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
	builder, err := dm.GetLogBuilderWithError()
	if err != nil {
		panic(err)
	}
	return builder
}

func (dm *DependencyManager) GetLogBuilderWithError() (aplog.Builder, error) {
	if dm.logBuilder == nil {
		logger, err := dm.GetRootLoggerWithError()
		if err != nil {
			return nil, err
		}
		dm.logBuilder = aplog.NewBuilder(logger)
	}

	return dm.logBuilder, nil
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
	logger, err := dm.GetRootLoggerWithError()
	if err != nil {
		panic(err)
	}
	return logger
}

func (dm *DependencyManager) GetRootLoggerWithError() (*slog.Logger, error) {
	dm.rootLoggerOnce.Do(func() {
		providers, err := dm.GetTelemetryWithError()
		if err != nil {
			dm.rootLoggerErr = err
			return
		}
		dm.rootLogger = aplog.WrapWithTelemetry(
			dm.GetConfigRoot().GetRootLogger(),
			providers,
			dm.GetConfigRoot().Telemetry,
		)
	})
	if dm.rootLoggerErr != nil {
		return nil, dm.rootLoggerErr
	}
	return dm.rootLogger, nil
}

func (dm *DependencyManager) GetLogger() *slog.Logger {
	logger, err := dm.GetLoggerWithError()
	if err != nil {
		panic(err)
	}
	return logger
}

func (dm *DependencyManager) GetLoggerWithError() (*slog.Logger, error) {
	if dm.logger == nil {
		b, err := dm.GetLogBuilderWithError()
		if err != nil {
			return nil, err
		}
		b = b.WithService(dm.serviceId)
		dm.logger = b.Build()
	}

	return dm.logger, nil
}

func (dm *DependencyManager) GetRedisClient() apredis.Client {
	client, err := dm.GetRedisClientWithError()
	if err != nil {
		panic(err)
	}
	return client
}

func (dm *DependencyManager) GetRedisClientWithError() (apredis.Client, error) {
	if dm.r == nil {
		telemetry, err := dm.GetTelemetryWithError()
		if err != nil {
			return nil, err
		}
		dm.r, err = apredis.NewForRoot(
			context.Background(),
			dm.GetConfig().GetRoot(),
			apredis.WithTelemetry(telemetry, dm.GetConfigRoot().Telemetry),
		)
		if err != nil {
			return nil, err
		}
	}

	return dm.r, nil
}

func (dm *DependencyManager) GetDatabase() database.DB {
	db, err := dm.GetDatabaseWithError()
	if err != nil {
		panic(err)
	}
	return db
}

func (dm *DependencyManager) GetDatabaseWithError() (database.DB, error) {
	if dm.db == nil {
		sqlDB, err := dm.GetSQLDBWithError()
		if err != nil {
			return nil, err
		}
		logger, err := dm.GetLoggerWithError()
		if err != nil {
			return nil, err
		}
		dm.db, err = database.NewService(
			sqlDB,
			dm.GetConfigRoot().Database.InnerVal,
			logger,
		)
		if err != nil {
			return nil, err
		}
	}

	return dm.db, nil
}

func (dm *DependencyManager) GetSQLDB() *sql.DB {
	db, err := dm.GetSQLDBWithError()
	if err != nil {
		panic(err)
	}
	return db
}

func (dm *DependencyManager) GetSQLDBWithError() (*sql.DB, error) {
	if dm.sqlDB == nil {
		db, err := dm.openConfiguredSQLDB()
		if err != nil {
			return nil, err
		}
		dm.sqlDB = db
	}
	return dm.sqlDB, nil
}

func (dm *DependencyManager) openConfiguredSQLDB() (*sql.DB, error) {
	root := dm.GetConfigRoot()
	if root == nil || root.Database == nil || root.Database.InnerVal == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	telemetry, err := dm.GetTelemetryWithError()
	if err != nil {
		return nil, err
	}

	switch cfg := root.Database.InnerVal.(type) {
	case *sconfig.DatabaseSqlite:
		if err := ensureSqliteDatabaseFile(cfg.Path); err != nil {
			return nil, err
		}
		return sqlh.OpenInstrumentedSQL(
			"sqlite3",
			cfg.GetDsn(),
			sqlh.DBSystemSQLite,
			sqlh.WithTelemetry(telemetry, root.Telemetry),
		)
	case *sconfig.DatabasePostgres:
		db, err := sqlh.OpenInstrumentedSQL(
			"pgx",
			cfg.GetDsn(),
			sqlh.DBSystemPostgreSQL,
			sqlh.WithTelemetry(telemetry, root.Telemetry),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres database '%s': %w", cfg.GetDsn(), err)
		}
		if err := applyPostgresPoolSettings(db, cfg); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to configure postgres database pool: %w", err)
		}
		return db, nil
	default:
		return nil, fmt.Errorf("database type not supported")
	}
}

func ensureSqliteDatabaseFile(configPath string) error {
	path := configPath
	if _, err := os.Stat(path); err != nil {
		expanded, expandErr := homedir.Expand(path)
		if expandErr != nil {
			return fmt.Errorf("failed to expand path; could not load sqlite database path '%s': %w", configPath, expandErr)
		}
		path = expanded
	}

	if _, err := os.Stat(path); err == nil {
		return nil
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not load sqlite database path '%s'; failed to create: %w", configPath, err)
	}
	return file.Close()
}

func applyPostgresPoolSettings(db *sql.DB, dbConfig *sconfig.DatabasePostgres) error {
	if db == nil || dbConfig == nil {
		return nil
	}

	ctx := context.Background()
	if dbConfig.MaxOpenConns != nil {
		value, err := databasePoolInt(ctx, "max_open_conns", dbConfig.MaxOpenConns)
		if err != nil {
			return err
		}
		db.SetMaxOpenConns(value)
	}
	if dbConfig.MaxIdleConns != nil {
		value, err := databasePoolInt(ctx, "max_idle_conns", dbConfig.MaxIdleConns)
		if err != nil {
			return err
		}
		db.SetMaxIdleConns(value)
	}
	if dbConfig.ConnMaxLifetime != nil {
		db.SetConnMaxLifetime(dbConfig.ConnMaxLifetime.Duration)
	}
	if dbConfig.ConnMaxIdleTime != nil {
		db.SetConnMaxIdleTime(dbConfig.ConnMaxIdleTime.Duration)
	}

	return nil
}

func databasePoolInt(ctx context.Context, name string, value *sconfig.IntegerValue) (int, error) {
	v, err := value.GetValue(ctx)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", name)
	}
	if v > int64(math.MaxInt) {
		return 0, fmt.Errorf("%s is too large: %d", name, v)
	}
	return int(v), nil
}

func (dm *DependencyManager) ShutdownDatabase() {
	if dm.sqlDB == nil {
		return
	}
	if err := dm.sqlDB.Close(); err != nil {
		dm.GetLogger().Warn("failed to close database", "error", err)
	}
	dm.sqlDB = nil
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
			sqlh.WithTelemetry(dm.GetTelemetry(), dm.GetConfigRoot().Telemetry),
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
			workflows.WithPostgresDB(dm.GetSQLDB()),
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
	telemetry, err := dm.GetTelemetryWithError()
	if err != nil {
		panic(err)
	}
	return telemetry
}

func (dm *DependencyManager) GetTelemetryWithError() (*aptelemetry.Providers, error) {
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
		return nil, fmt.Errorf("failed to initialise telemetry: %w", dm.telemetryErr)
	}

	return dm.telemetry, nil
}

func (dm *DependencyManager) GetDataEncryptionKeyTelemetry() *encrypt.DataEncryptionKeyTelemetry {
	telemetry, err := dm.GetDataEncryptionKeyTelemetryWithError()
	if err != nil {
		panic(err)
	}
	return telemetry
}

func (dm *DependencyManager) GetDataEncryptionKeyTelemetryWithError() (*encrypt.DataEncryptionKeyTelemetry, error) {
	dm.dataEncryptionKeyTelemetryOnce.Do(func() {
		providers, err := dm.GetTelemetryWithError()
		if err != nil {
			dm.dataEncryptionKeyTelemetryErr = err
			return
		}
		tel, err := encrypt.NewDataEncryptionKeyTelemetry(
			providers,
			dm.GetConfigRoot().Telemetry,
		)
		if err != nil {
			dm.dataEncryptionKeyTelemetryErr = err
			return
		}
		dm.dataEncryptionKeyTelemetry = tel
	})

	if dm.dataEncryptionKeyTelemetryErr != nil {
		return nil, fmt.Errorf("failed to initialise data encryption key telemetry: %w", dm.dataEncryptionKeyTelemetryErr)
	}

	return dm.dataEncryptionKeyTelemetry, nil
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
			core.WithWorkflowClient(dm.GetWorkflowRuntime().Client()),
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
