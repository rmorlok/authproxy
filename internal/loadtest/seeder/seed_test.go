package seeder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfileSmoke(t *testing.T) {
	profile, err := LoadProfile(filepath.Join("..", "..", "..", "loadtest", "profiles", "smoke.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "smoke", profile.Name)
	assert.Equal(t, 10, profile.TenantNamespaceCount())
	assert.Equal(t, 10, profile.Objects.Connections)
	assert.Equal(t, 2, profile.Objects.StaleSetupConnections)
	assert.Equal(t, 0, profile.DefaultOAuthExpiringPercent())
	assert.Equal(t, 0, profile.DefaultPeriodicProbePercent())
}

func TestSeedSmokeProfile(t *testing.T) {
	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	enc := encrypt.NewFakeEncryptService(false)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	oauthExpiringPercent := 40
	periodicProbePercent := 20
	staleSetupConnections := 2

	result, err := Seed(context.Background(), Options{
		Profile: Profile{
			Name: "smoke",
			Objects: ProfileObjects{
				Namespaces:  3,
				Connections: 5,
			},
		},
		DB:                    db,
		Encrypt:               enc,
		ProviderBaseURL:       "http://provider.test",
		OAuthExpiringPercent:  &oauthExpiringPercent,
		PeriodicProbePercent:  &periodicProbePercent,
		StaleSetupConnections: &staleSetupConnections,
		VerifySamples:         3,
		Now:                   now,
	})
	require.NoError(t, err)

	assert.Equal(t, "root.loadtest.smoke", result.BaseNamespace)
	assert.Equal(t, 3, result.CreatedNamespaces)
	assert.Equal(t, 3, result.UpsertedActors)
	assert.Equal(t, 5, result.CreatedConnections)
	assert.Equal(t, 0, result.ExistingConnections)
	assert.Equal(t, 5, result.UpsertedOAuthTokens)
	assert.Equal(t, 1, result.ProbeEnabledConnections)
	assert.Equal(t, 2, result.CreatedStaleSetups)
	assert.Equal(t, 0, result.ExistingStaleSetups)
	require.Len(t, result.VerifiedSamples, 3)
	require.Len(t, result.Connections, 5)
	require.Len(t, result.StaleSetups, 2)

	first := result.Connections[0]
	assert.Equal(t, "rt_"+first.ConnectionID.String(), first.RefreshToken)
	assert.True(t, first.ProbeEnabled)
	assert.Equal(t, now.Add(time.Minute), first.AccessTokenExpiresAt)

	last := result.Connections[4]
	assert.False(t, last.ProbeEnabled)
	assert.Equal(t, now.Add(24*time.Hour), last.AccessTokenExpiresAt)

	gotConnection, err := db.GetConnection(context.Background(), first.ConnectionID)
	require.NoError(t, err)
	assert.Equal(t, first.Namespace, gotConnection.Namespace)

	gotToken, err := db.GetOAuth2Token(context.Background(), first.ConnectionID)
	require.NoError(t, err)
	assert.Equal(t, first.ConnectionID, gotToken.ConnectionId)

	gotStaleSetup, err := db.GetConnection(context.Background(), result.StaleSetups[0].ConnectionID)
	require.NoError(t, err)
	assert.Equal(t, database.ConnectionStateSetup, gotStaleSetup.State)
	require.NotNil(t, gotStaleSetup.SetupStep)
	assert.Equal(t, "loadtest_stale_setup", gotStaleSetup.SetupStep.String())
	assert.Equal(t, "true", gotStaleSetup.Labels["loadtest.authproxy.io/stale-setup"])
}

func TestSeedWritesArtifactsAndIsConnectionIdempotent(t *testing.T) {
	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	enc := encrypt.NewFakeEncryptService(false)
	profile := Profile{
		Name: "smoke",
		Objects: ProfileObjects{
			Namespaces:            2,
			Connections:           2,
			StaleSetupConnections: 1,
		},
	}

	first, err := Seed(context.Background(), Options{
		Profile:       profile,
		DB:            db,
		Encrypt:       enc,
		VerifySamples: 2,
		Now:           time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, first.CreatedConnections)
	assert.Equal(t, 1, first.CreatedStaleSetups)

	second, err := Seed(context.Background(), Options{
		Profile:       profile,
		DB:            db,
		Encrypt:       enc,
		VerifySamples: 2,
		Now:           time.Date(2026, 7, 13, 12, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.Equal(t, 0, second.CreatedConnections)
	assert.Equal(t, 2, second.ExistingConnections)
	assert.Equal(t, 0, second.CreatedStaleSetups)
	assert.Equal(t, 1, second.ExistingStaleSetups)

	runDir := t.TempDir()
	require.NoError(t, WriteArtifacts(runDir, second))

	connectionsCSV, err := os.ReadFile(filepath.Join(runDir, "datasets", "connections.csv"))
	require.NoError(t, err)
	assert.Contains(t, string(connectionsCSV), "connection_id,namespace,actor_id")
	assert.Contains(t, string(connectionsCSV), "cxn_lt_smoke_000000001")

	staleCSV, err := os.ReadFile(filepath.Join(runDir, "datasets", "stale_setup_connections.csv"))
	require.NoError(t, err)
	assert.Contains(t, string(staleCSV), "cxn_lt_smoke_stale_000000001")

	summary, err := os.ReadFile(filepath.Join(runDir, "seed-summary.json"))
	require.NoError(t, err)
	assert.Contains(t, string(summary), `"existing_connections": 2`)
	assert.Contains(t, string(summary), `"existing_stale_setup_connections": 1`)

	plan, err := os.ReadFile(filepath.Join(runDir, "seed-plan.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(plan), "AuthProxy load-test seed summary")
	assert.Contains(t, string(plan), "datasets/stale_setup_connections.csv")
}
