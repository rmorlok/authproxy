package app_metrics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/rmorlok/authproxy/internal/migration"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestSQLMigrateUsesAppMetricsSchemaMigrationsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.sqlite3")
	cfg := &sconfig.Database{InnerVal: &sconfig.DatabaseSqlite{Path: dbPath}}

	db, err := sql.Open(cfg.GetDriver(), cfg.GetDsn())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE schema_migrations (version uint64, dirty bool);
		INSERT INTO schema_migrations (version, dirty) VALUES (999, false);
	`)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(
		context.Background(),
		cfg,
		newTestHarnessLogger(),
		migration.DirectionUp,
		nil,
	))

	var mainVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&mainVersion))
	require.Equal(t, 999, mainVersion)

	var appMetricsVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM app_metrics_schema_migrations").Scan(&appMetricsVersion))
	require.Greater(t, appMetricsVersion, 0)

	status := MigrationStatus(context.Background(), cfg)
	require.Equal(t, migration.StateCurrent, status.State)
	require.Equal(t, uint(5), status.AvailableVersion)
	require.Equal(t, uint(5), *status.CurrentVersion)
}

func TestMigrationStatusCurrentForConfiguredProvider(t *testing.T) {
	store, _, _ := MustNewBlankRequestEventsStore(t)
	var cfg *sconfig.Database
	switch concrete := store.(type) {
	case *sqlRecordStore:
		cfg = concrete.cfg
	case *clickhouseRecordStore:
		cfg = &sconfig.Database{InnerVal: concrete.cfg}
	default:
		t.Fatalf("unexpected record store type %T", store)
	}

	status := MigrationStatus(context.Background(), cfg)
	require.NoError(t, status.Err)
	require.True(t, status.Compatible())
	require.Equal(t, status.AvailableVersion, *status.CurrentVersion)
}
