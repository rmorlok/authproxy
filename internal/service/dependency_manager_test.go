package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPingAndRunPings_AllOk(t *testing.T) {
	dm := &DependencyManager{
		pings: make(map[string]PingFunc),
	}

	dm.RegisterPing("db", func(ctx context.Context) bool { return true })
	dm.RegisterPing("redis", func(ctx context.Context) bool { return true })

	results, allOk := dm.RunPings(context.Background())
	assert.True(t, allOk)
	assert.True(t, results["db"])
	assert.True(t, results["redis"])
	assert.Len(t, results, 2)
}

func TestRunPings_OneFailure(t *testing.T) {
	dm := &DependencyManager{
		pings: make(map[string]PingFunc),
	}

	dm.RegisterPing("db", func(ctx context.Context) bool { return true })
	dm.RegisterPing("redis", func(ctx context.Context) bool { return false })

	results, allOk := dm.RunPings(context.Background())
	assert.False(t, allOk)
	assert.True(t, results["db"])
	assert.False(t, results["redis"])
}

func TestRunPings_Empty(t *testing.T) {
	dm := &DependencyManager{
		pings: make(map[string]PingFunc),
	}

	results, allOk := dm.RunPings(context.Background())
	assert.True(t, allOk)
	assert.Empty(t, results)
}

func TestRunPings_ContextCancellation(t *testing.T) {
	dm := &DependencyManager{
		pings: make(map[string]PingFunc),
	}

	dm.RegisterPing("slow", func(ctx context.Context) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(5 * time.Second):
			return true
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	results, allOk := dm.RunPings(ctx)
	assert.False(t, allOk)
	assert.False(t, results["slow"])
}

func TestApplyPostgresPoolSettings(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = applyPostgresPoolSettings(db, &config.DatabasePostgres{
		MaxOpenConns:    common.NewIntegerValueDirectInline(7),
		MaxIdleConns:    common.NewIntegerValueDirectInline(3),
		ConnMaxLifetime: &config.HumanDuration{Duration: 30 * time.Minute},
		ConnMaxIdleTime: &config.HumanDuration{Duration: 5 * time.Minute},
	})
	require.NoError(t, err)

	require.Equal(t, 7, db.Stats().MaxOpenConnections)
}

func TestApplyPostgresPoolSettingsRejectsInvalidValues(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = applyPostgresPoolSettings(db, &config.DatabasePostgres{
		MaxOpenConns: common.NewIntegerValueDirectInline(-1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_open_conns")
}
