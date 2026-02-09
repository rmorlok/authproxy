package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/golangmigrator"
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

var postgresTestLimiter = make(chan struct{}, getEnvIntDefault("POSTGRES_TEST_MAX_PARALLEL", 4))

// MustApplyBlankTestDbConfig applies a test database configuration to the specified config root. The database
// is guaranteed to be blank and migrated. This method uses a temp file so that the database will be eventually
// cleaned up after the process exits. Note that the configuration in the root will be modified for the database
// and populated for the GlobalAESKey if it is not already populated.
//
// To support debugging tests by inspecting the SQLite database, if the SQLITE_TEST_DATABASE_PATH env var is set
// this method will use the database at that path. It will delete the existing file at that path to recreate unless
// the SQLITE_TEST_DATABASE_PATH_CLEAR env var is set to false.
//
// To run tests against Postgres, set AUTH_PROXY_TEST_DATABASE_PROVIDER=postgres and configure the
// connection with POSTGRES_TEST_HOST, POSTGRES_TEST_PORT, POSTGRES_TEST_USER, POSTGRES_TEST_PASSWORD,
// POSTGRES_TEST_DATABASE, and POSTGRES_TEST_OPTIONS. You can also tune
// POSTGRES_TEST_MAX_PARALLEL and POSTGRES_TEST_MAX_CONNS to reduce connection pressure.
//
// Parameters:
// - t: the test instance used for naming and cleanup
// - cfg: the config to apply the database config to. This may be nil, in which case a new config is created. This method will overwrite the existing config.
//
// Returns:
// - the config with information populated for the database. If a config was passed in, the same value is returned with data populated.
// - a database instance configured with the specified root. This database can be used directly, or if the root used again, it will connect to the same database instance.
func MustApplyBlankTestDbConfig(t testing.TB, cfg config.C) (config.C, DB) {
	c, db, _ := MustApplyBlankTestDbConfigRaw(t, cfg)
	return c, db
}

func MustApplyBlankTestDbConfigRaw(t testing.TB, cfg config.C) (config.C, DB, *sql.DB) {
	t.Helper()

	// Optionally load the dotenv file as to force tests into postgres using environment variables while debugging
	_ = godotenv.Load()

	if cfg == nil {
		cfg = config.FromRoot(&sconfig.Root{})
	}

	root := cfg.GetRoot()

	if root == nil {
		panic("No root in config")
	}

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_PROXY_TEST_DATABASE_PROVIDER")))
	if provider == "" {
		provider = "sqlite"
	}

	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &sconfig.KeyData{InnerVal: &sconfig.KeyDataRandomBytes{}}
	}

	switch provider {
	case "postgres":
		return mustApplyBlankPostgresTestDbConfig(t, cfg)
	default:
		return mustApplyBlankSqliteTestDbConfig(t, cfg)
	}
}

func mustApplyBlankSqliteTestDbConfig(t testing.TB, cfg config.C) (config.C, DB, *sql.DB) {
	t.Helper()

	root := cfg.GetRoot()
	testName := t.Name()
	if testName != "" {
		testName = testName + "-"
	}

	tempFilePath := os.Getenv("SQLITE_TEST_DATABASE_PATH")
	if tempFilePath != "" {
		clearEnv := os.Getenv("SQLITE_TEST_DATABASE_PATH_CLEAR")
		if clearEnv != "false" {
			_ = os.Remove(tempFilePath)
		}
	} else {
		tempFilePath = filepath.Join(
			os.TempDir(),
			fmt.Sprintf("authproxy-tests/db/%s-%d-%s%s.sqlite3", time.Now().Format("2006-01-02T15-04-05"), os.Getpid(), testName, uuid.New().String()),
		)
	}

	dirPath := filepath.Dir(tempFilePath)
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		t.Fatalf("failed to create sqlite test directory: %v", err)
	}

	if _, err := os.Create(tempFilePath); err != nil {
		t.Fatalf("failed to create sqlite test database: %v", err)
	}

	root.Database = &sconfig.DatabaseSqlite{
		Path: tempFilePath,
	}

	db, err := NewConnectionForRoot(root, root.GetRootLogger())
	if err != nil {
		t.Fatalf("failed to connect sqlite test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.(*service).db.Close()
	})

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate sqlite test database: %v", err)
	}

	if err := db.CreateNamespace(context.Background(), &Namespace{
		Path:  "root",
		State: NamespaceStateActive,
	}); err != nil {
		t.Fatalf("failed to create root namespace: %v", err)
	}

	return cfg, db, db.(*service).db
}

func mustApplyBlankPostgresTestDbConfig(t testing.TB, cfg config.C) (config.C, DB, *sql.DB) {
	t.Helper()

	root := cfg.GetRoot()

	postgresTestLimiter <- struct{}{}
	defer func() { <-postgresTestLimiter }()

	adminConfig := pgtestdb.Config{
		DriverName: "pgx",
		User:       util.GetEnvDefault("POSTGRES_TEST_USER", "postgres"),
		Password:   util.GetEnvDefault("POSTGRES_TEST_PASSWORD", "postgres"),
		Host:       util.GetEnvDefault("POSTGRES_TEST_HOST", "localhost"),
		Port:       util.GetEnvDefault("POSTGRES_TEST_PORT", "5432"),
		Database:   util.GetEnvDefault("POSTGRES_TEST_DATABASE", "postgres"),
		Options:    util.GetEnvDefault("POSTGRES_TEST_OPTIONS", "sslmode=disable"),
	}

	migrator := golangmigrator.New(
		"migrations/postgres",
		golangmigrator.WithFS(migrationsFs),
	)

	testDbConfig := pgtestdb.Custom(t, adminConfig, migrator)
	rawDb, err := testDbConfig.Connect()
	if err != nil {
		t.Fatalf("failed to connect postgres test database: %v", err)
	}
	maxConns := getEnvIntDefault("POSTGRES_TEST_MAX_CONNS", 2)
	rawDb.SetMaxOpenConns(maxConns)
	rawDb.SetMaxIdleConns(maxConns)
	rawDb.SetConnMaxLifetime(2 * time.Minute)
	t.Cleanup(func() {
		_ = rawDb.Close()
	})

	port, err := strconv.Atoi(testDbConfig.Port)
	if err != nil {
		port = 5432
	}

	sslMode := ""
	params := map[string]string{}
	if testDbConfig.Options != "" {
		query, err := url.ParseQuery(testDbConfig.Options)
		if err == nil {
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

	root.Database = &sconfig.DatabasePostgres{
		Provider: sconfig.DatabaseProviderPostgres,
		Host:     testDbConfig.Host,
		Port:     port,
		User:     testDbConfig.User,
		Password: testDbConfig.Password,
		Database: testDbConfig.Database,
		SSLMode:  sslMode,
		Params:   params,
	}

	db := &service{
		cfg:       root.Database,
		sq:        sq.StatementBuilder.PlaceholderFormat(root.Database.GetPlaceholderFormat()),
		db:        rawDb,
		secretKey: root.SystemAuth.GlobalAESKey,
		logger:    root.GetRootLogger(),
	}

	if err := db.CreateNamespace(context.Background(), &Namespace{
		Path:  "root",
		State: NamespaceStateActive,
	}); err != nil {
		t.Fatalf("failed to create root namespace: %v", err)
	}

	return cfg, db, rawDb
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
