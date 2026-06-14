package app_metrics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

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

	store, _, _ := buildSqlStorePairNoMigrate(t, cfg)
	require.NoError(t, store.(*sqlRecordStore).Migrate(context.Background()))

	var mainVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&mainVersion))
	require.Equal(t, 999, mainVersion)

	var appMetricsVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM app_metrics_schema_migrations").Scan(&appMetricsVersion))
	require.Greater(t, appMetricsVersion, 0)
}
