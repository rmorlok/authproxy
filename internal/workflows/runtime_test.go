package workflows

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestMigrateSqliteAndRuntimePing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflows.sqlite")
	root := &config.Root{
		Database: &config.Database{
			InnerVal: &config.DatabaseSqlite{
				Provider: config.DatabaseProviderSqlite,
				Path:     dbPath,
			},
		},
	}
	logger := slog.New(slog.DiscardHandler)

	require.NoError(t, Migrate(root, logger))

	db, err := sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)
	defer db.Close()

	var tableName string
	err = db.QueryRowContext(
		context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'instances'`,
	).Scan(&tableName)
	require.NoError(t, err)
	require.Equal(t, "instances", tableName)

	runtime, err := NewRuntime(root, nil, logger)
	require.NoError(t, err)
	defer runtime.Close()

	require.True(t, runtime.Ping(context.Background()))
}

func TestMigrateSqliteUsesDedicatedMigrationsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflows.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)

	_, err = db.ExecContext(
		context.Background(),
		`CREATE TABLE schema_migrations (version uint64, dirty bool)`,
	)
	require.NoError(t, err)
	_, err = db.ExecContext(
		context.Background(),
		`CREATE UNIQUE INDEX version_unique ON schema_migrations (version)`,
	)
	require.NoError(t, err)
	_, err = db.ExecContext(
		context.Background(),
		`INSERT INTO schema_migrations (version, dirty) VALUES (10, false)`,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	root := &config.Root{
		Database: &config.Database{
			InnerVal: &config.DatabaseSqlite{
				Provider: config.DatabaseProviderSqlite,
				Path:     dbPath,
			},
		},
	}

	require.NoError(t, Migrate(root, slog.New(slog.DiscardHandler)))

	db, err = sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)
	defer db.Close()

	var mainVersion int
	err = db.QueryRowContext(
		context.Background(),
		`SELECT version FROM schema_migrations`,
	).Scan(&mainVersion)
	require.NoError(t, err)
	require.Equal(t, 10, mainVersion)

	var workflowVersion int
	err = db.QueryRowContext(
		context.Background(),
		`SELECT version FROM authproxy_workflows_schema_migrations`,
	).Scan(&workflowVersion)
	require.NoError(t, err)
	require.Positive(t, workflowVersion)
}
