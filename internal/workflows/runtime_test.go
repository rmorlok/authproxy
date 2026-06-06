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
