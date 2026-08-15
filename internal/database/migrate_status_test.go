package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestMigrationStatusAndDirectionsSQLite(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Database{InnerVal: &config.DatabaseSqlite{
		Provider: config.DatabaseProviderSqlite,
		Path:     filepath.Join(t.TempDir(), "database.sqlite"),
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	missing := MigrationStatus(ctx, cfg)
	require.Equal(t, migration.StateMissing, missing.State)
	require.Equal(t, uint(16), missing.AvailableVersion)

	require.NoError(t, RunMigrations(ctx, cfg, logger, migration.DirectionUp, nil))
	current := MigrationStatus(ctx, cfg)
	require.True(t, current.Compatible())
	require.Equal(t, uint(16), *current.CurrentVersion)

	target := uint(15)
	require.NoError(t, RunMigrations(ctx, cfg, logger, migration.DirectionDown, &target))
	behind := MigrationStatus(ctx, cfg)
	require.Equal(t, migration.StateBehind, behind.State)
	require.Equal(t, target, *behind.CurrentVersion)

	require.NoError(t, RunMigrations(ctx, cfg, logger, migration.DirectionUp, nil))
	require.True(t, MigrationStatus(ctx, cfg).Compatible())
}

func TestMigrationStatusCurrentForConfiguredProvider(t *testing.T) {
	cfg, _, _ := MustApplyBlankTestDbConfigRaw(t, nil)
	status := MigrationStatus(context.Background(), cfg.GetRoot().Database)
	require.NoError(t, status.Err)
	require.True(t, status.Compatible())
	require.Equal(t, status.AvailableVersion, *status.CurrentVersion)
}
