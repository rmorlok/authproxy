package request_log

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/golangmigrator"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// TestProviderEnvVar names the env variable that selects which backend the
// request_log test harness should use. The selector is independent of the
// main database's AUTH_PROXY_TEST_DATABASE_PROVIDER so callers can mix the
// two (e.g. main DB on sqlite, request_log on clickhouse).
const TestProviderEnvVar = "AUTH_PROXY_REQUEST_LOG_TEST_DATABASE_PROVIDER"

var (
	postgresRequestLogTestLimiter   = make(chan struct{}, getEnvIntDefault("POSTGRES_TEST_MAX_PARALLEL", 4))
	clickhouseRequestLogTestLimiter = make(chan struct{}, getEnvIntDefault("CLICKHOUSE_TEST_MAX_PARALLEL", 2))
)

// MustNewBlankRequestLogStore returns a fresh, migrated RecordStore +
// RecordRetriever backed by whichever provider AUTH_PROXY_REQUEST_LOG_TEST_DATABASE_PROVIDER
// names (sqlite|postgres|clickhouse; default sqlite). The raw *sql.DB is also
// returned so tests can issue direct queries when convenient. Cleanup is
// registered with t.
//
// Postgres reuses the existing POSTGRES_TEST_* connection pool (host, port,
// user, password, admin database, options) — pgtestdb creates an isolated
// per-test database against that shared instance. ClickHouse uses
// CLICKHOUSE_TEST_{HOST,PORT,USER,PASSWORD,DATABASE,MAX_PARALLEL,MAX_CONNS}
// and creates a unique per-test database, dropped on cleanup.
func MustNewBlankRequestLogStore(t testing.TB) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	util.LoadDotEnv()

	provider := strings.ToLower(strings.TrimSpace(os.Getenv(TestProviderEnvVar)))
	if provider == "" {
		provider = "sqlite"
	}

	switch provider {
	case "sqlite":
		return mustNewBlankSqliteRequestLogStore(t)
	case "postgres":
		return mustNewBlankPostgresRequestLogStore(t)
	case "clickhouse":
		return mustNewBlankClickhouseRequestLogStore(t)
	default:
		t.Fatalf("unknown request_log test provider %q (expected sqlite, postgres, or clickhouse)", provider)
		return nil, nil, nil
	}
}

func newTestCursorEncryptor() pagination.CursorEncryptor {
	return pagination.NewDefaultCursorEncryptor([]byte("0123456789abcdef0123456789abcdef"))
}

func newTestHarnessLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func mustNewBlankSqliteRequestLogStore(t testing.TB) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	testName := t.Name()
	if testName != "" {
		testName = testName + "-"
	}
	tempFilePath := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("authproxy-tests/request_log/%s-%d-%s%s.sqlite3",
			time.Now().Format("2006-01-02T15-04-05"), os.Getpid(), testName, uuid.New().String()),
	)
	if err := os.MkdirAll(filepath.Dir(tempFilePath), os.ModePerm); err != nil {
		t.Fatalf("failed to create sqlite test directory: %v", err)
	}
	if _, err := os.Create(tempFilePath); err != nil {
		t.Fatalf("failed to create sqlite test database: %v", err)
	}

	cfg := &sconfig.Database{InnerVal: &sconfig.DatabaseSqlite{Path: tempFilePath}}
	return mustBuildSqlStorePair(t, cfg)
}

func mustNewBlankPostgresRequestLogStore(t testing.TB) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	postgresRequestLogTestLimiter <- struct{}{}
	t.Cleanup(func() { <-postgresRequestLogTestLimiter })

	adminConfig := pgtestdb.Config{
		DriverName:                "pgx",
		User:                      util.GetEnvDefault("POSTGRES_TEST_USER", "postgres"),
		Password:                  util.GetEnvDefault("POSTGRES_TEST_PASSWORD", "postgres"),
		Host:                      util.GetEnvDefault("POSTGRES_TEST_HOST", "localhost"),
		Port:                      util.GetEnvDefault("POSTGRES_TEST_PORT", "5432"),
		Database:                  util.GetEnvDefault("POSTGRES_TEST_DATABASE", "postgres"),
		Options:                   util.GetEnvDefault("POSTGRES_TEST_OPTIONS", "sslmode=disable"),
		ForceTerminateConnections: true,
	}

	migrator := golangmigrator.New(
		"migrations/postgres",
		golangmigrator.WithFS(httpLogMigrationsFs),
	)

	testDbConfig := pgtestdb.Custom(t, adminConfig, migrator)

	port, err := strconv.Atoi(testDbConfig.Port)
	if err != nil {
		port = 5432
	}

	sslMode := ""
	params := map[string]string{}
	if testDbConfig.Options != "" {
		if query, err := url.ParseQuery(testDbConfig.Options); err == nil {
			for key, values := range query {
				if len(values) == 0 {
					continue
				}
				if key == "sslmode" {
					sslMode = values[0]
				} else {
					params[key] = values[0]
				}
			}
		}
	}

	cfg := &sconfig.Database{InnerVal: &sconfig.DatabasePostgres{
		Provider: sconfig.DatabaseProviderPostgres,
		Host:     scommon.NewStringValueDirectInline(testDbConfig.Host),
		Port:     scommon.NewIntegerValueDirectInline(int64(port)),
		User:     scommon.NewStringValueDirectInline(testDbConfig.User),
		Password: scommon.NewStringValueDirectInline(testDbConfig.Password),
		Database: scommon.NewStringValueDirectInline(testDbConfig.Database),
		SSLMode:  scommon.NewStringValueDirectInline(sslMode),
		Params:   params,
	}}

	// pgtestdb already ran the migrator against the per-test database, so the
	// SQL store can be built without re-migrating.
	return buildSqlStorePairNoMigrate(t, cfg)
}

func mustNewBlankClickhouseRequestLogStore(t testing.TB) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	clickhouseRequestLogTestLimiter <- struct{}{}
	t.Cleanup(func() { <-clickhouseRequestLogTestLimiter })

	host := util.GetEnvDefault("CLICKHOUSE_TEST_HOST", "localhost")
	port := util.GetEnvDefault("CLICKHOUSE_TEST_PORT", "8123")
	user := util.GetEnvDefault("CLICKHOUSE_TEST_USER", "default")
	password := util.GetEnvDefault("CLICKHOUSE_TEST_PASSWORD", "")
	adminDb := util.GetEnvDefault("CLICKHOUSE_TEST_DATABASE", "default")

	portInt, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("invalid CLICKHOUSE_TEST_PORT %q: %v", port, err)
	}

	addr := fmt.Sprintf("%s:%d", host, portInt)
	protocol := clickhouse.HTTP
	if portInt == 9000 || portInt == 9440 {
		protocol = clickhouse.Native
	}

	openDb := func(databaseName string) *sql.DB {
		opts := &clickhouse.Options{
			Addr:     []string{addr},
			Protocol: protocol,
			Auth: clickhouse.Auth{
				Database: databaseName,
				Username: user,
				Password: password,
			},
		}
		return sql.OpenDB(clickhouse.Connector(opts))
	}

	adminConn := openDb(adminDb)
	t.Cleanup(func() { _ = adminConn.Close() })
	if err := adminConn.PingContext(context.Background()); err != nil {
		t.Fatalf("failed to connect to clickhouse admin db %q at %s: %v", adminDb, addr, err)
	}

	testDbName := fmt.Sprintf("authproxy_rl_test_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	if _, err := adminConn.ExecContext(context.Background(),
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", testDbName)); err != nil {
		t.Fatalf("failed to create clickhouse test database %q: %v", testDbName, err)
	}
	t.Cleanup(func() {
		_, _ = adminConn.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", testDbName))
	})

	cfg := &sconfig.Database{InnerVal: &sconfig.DatabaseClickhouse{
		Provider:  sconfig.DatabaseProviderClickhouse,
		Addresses: []string{addr},
		Database:  scommon.NewStringValueDirectInline(testDbName),
		User:      scommon.NewStringValueDirectInline(user),
		Password:  scommon.NewStringValueDirectInline(password),
	}}

	logger := newTestHarnessLogger()
	store := NewClickhouseRecordStore(cfg, logger).(*clickhouseRecordStore)
	t.Cleanup(func() { _ = store.db.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate clickhouse request_log test database: %v", err)
	}

	retriever := NewClickhouseRecordRetriever(cfg, newTestCursorEncryptor(), logger).(*clickhouseRecordRetriever)
	t.Cleanup(func() { _ = retriever.db.Close() })

	return store, retriever, store.db
}

// mustBuildSqlStorePair constructs an sqlRecordStore + sqlRecordRetriever and
// runs migrations. Used by the sqlite path; the postgres path uses pgtestdb's
// migrator and calls buildSqlStorePairNoMigrate instead.
func mustBuildSqlStorePair(t testing.TB, cfg *sconfig.Database) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	store, retriever, rawDb := buildSqlStorePairNoMigrate(t, cfg)
	if err := store.(*sqlRecordStore).Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate request_log test database: %v", err)
	}
	return store, retriever, rawDb
}

func buildSqlStorePairNoMigrate(t testing.TB, cfg *sconfig.Database) (RecordStore, RecordRetriever, *sql.DB) {
	t.Helper()

	logger := newTestHarnessLogger()
	store := NewSqlRecordStore(cfg, logger).(*sqlRecordStore)
	t.Cleanup(func() { _ = store.db.Close() })

	retriever := NewSqlRecordRetriever(cfg, newTestCursorEncryptor(), logger).(*sqlRecordRetriever)
	t.Cleanup(func() { _ = retriever.db.Close() })

	return store, retriever, store.db
}

func getEnvIntDefault(key string, fallback int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	if parsed < 1 {
		return fallback
	}
	return parsed
}
