package workflows

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rmorlok/authproxy/internal/schema/common"
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

func TestNewRuntimePostgresBorrowedDBIsNotClosed(t *testing.T) {
	root := &config.Root{
		Database: &config.Database{
			InnerVal: &config.DatabasePostgres{
				Provider: config.DatabaseProviderPostgres,
				Host:     common.NewStringValueDirectInline("localhost"),
				Port:     common.NewIntegerValueDirectInline(5432),
				User:     common.NewStringValueDirectInline("authproxy"),
				Password: common.NewStringValueDirectInline("authproxy"),
				Database: common.NewStringValueDirectInline("authproxy"),
			},
		},
	}
	logger := slog.New(slog.DiscardHandler)

	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	runtime, err := NewRuntime(root, nil, logger, WithPostgresDB(db))
	require.NoError(t, err)

	require.NoError(t, runtime.Close())
	require.NoError(t, db.Ping())
}
