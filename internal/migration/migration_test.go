package migration

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestLatestVersion(t *testing.T) {
	version, err := LatestVersion(fstest.MapFS{
		"migrations/000001_init.up.sql":   {},
		"migrations/000001_init.down.sql": {},
		"migrations/000007_latest.up.sql": {},
	}, "migrations")
	require.NoError(t, err)
	require.Equal(t, uint(7), version)
}

func TestInspectMissingSQLiteIsReadOnly(t *testing.T) {
	path := t.TempDir() + "/missing.sqlite"
	cfg := sqliteConfig(path)

	status := Inspect(context.Background(), TargetMainDatabase, cfg, "schema_migrations", 4)

	require.Equal(t, StateMissing, status.State)
	require.Nil(t, status.CurrentVersion)
	require.NoFileExists(t, path)
}

func TestInspectSQLiteStates(t *testing.T) {
	tests := []struct {
		name      string
		current   uint
		dirty     bool
		available uint
		expected  State
	}{
		{name: "current", current: 4, available: 4, expected: StateCurrent},
		{name: "behind", current: 3, available: 4, expected: StateBehind},
		{name: "ahead", current: 5, available: 4, expected: StateAhead},
		{name: "dirty", current: 4, dirty: true, available: 4, expected: StateDirty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir() + "/status.sqlite"
			db, err := sql.Open("sqlite3", path)
			require.NoError(t, err)
			_, err = db.Exec(`CREATE TABLE schema_migrations (version uint64, dirty bool)`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)`, tt.current, tt.dirty)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			status := Inspect(context.Background(), TargetMainDatabase, sqliteConfig(path), "schema_migrations", tt.available)

			require.NoError(t, status.Err)
			require.Equal(t, tt.expected, status.State)
			require.NotNil(t, status.CurrentVersion)
			require.Equal(t, tt.current, *status.CurrentVersion)
			require.Equal(t, tt.dirty, status.Dirty)
		})
	}
}

func TestValidateRequest(t *testing.T) {
	current := uint(4)
	status := NewStatus(TargetMainDatabase, config.DatabaseProviderSqlite, &current, 7, false)

	tooLow := uint(3)
	require.Error(t, ValidateRequest(status, DirectionUp, &tooLow))
	tooHigh := uint(8)
	require.Error(t, ValidateRequest(status, DirectionUp, &tooHigh))
	validUp := uint(6)
	require.NoError(t, ValidateRequest(status, DirectionUp, &validUp))
	validDown := uint(2)
	require.NoError(t, ValidateRequest(status, DirectionDown, &validDown))
}

func sqliteConfig(path string) *config.Database {
	return &config.Database{InnerVal: &config.DatabaseSqlite{
		Provider: config.DatabaseProviderSqlite,
		Path:     path,
	}}
}
