package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	apjwt "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apctx"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestParseTokenDuration(t *testing.T) {
	t.Parallel()

	t.Run("supports days", func(t *testing.T) {
		d, err := parseTokenDuration("90d")
		require.NoError(t, err)
		require.Equal(t, 90*24*time.Hour, d)
	})

	t.Run("keeps standard duration support", func(t *testing.T) {
		d, err := parseTokenDuration("36h")
		require.NoError(t, err)
		require.Equal(t, 36*time.Hour, d)
	})

	t.Run("rejects invalid duration", func(t *testing.T) {
		_, err := parseTokenDuration("forever")
		require.Error(t, err)
	})
}

func TestGrafanaPresetPermissions(t *testing.T) {
	t.Parallel()

	t.Run("aggregate preset grants metric schema query and variable lists", func(t *testing.T) {
		permissions, err := grafanaPresetPermissions("aggregate")
		require.NoError(t, err)

		require.Contains(t, permissions, aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"app-metrics"},
			Verbs:     []string{"schema", "query"},
		})
		require.Contains(t, permissions, aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"namespaces", "connectors", "connections", "actors", "rate_limits"},
			Verbs:     []string{"list"},
		})
		require.Contains(t, permissions, aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"connectors"},
			Verbs:     []string{"list/versions"},
		})
		require.NotContains(t, permissions, aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"request-events"},
			Verbs:     []string{"list"},
		})
	})

	t.Run("logs preset includes request events", func(t *testing.T) {
		permissions, err := grafanaPresetPermissions("logs")
		require.NoError(t, err)

		require.Contains(t, permissions, aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"request-events"},
			Verbs:     []string{"list"},
		})
	})

	t.Run("unknown preset returns error", func(t *testing.T) {
		_, err := grafanaPresetPermissions("wide-open")
		require.Error(t, err)
	})
}

func TestReadPermissionsFile(t *testing.T) {
	t.Parallel()

	t.Run("direct array", func(t *testing.T) {
		path := writeTestFile(t, `
- namespace: root.alpha
  resources: [connections]
  verbs: [list]
`)

		permissions, err := readPermissionsFile(path)
		require.NoError(t, err)
		require.Equal(t, []aschema.Permission{
			{
				Namespace: "root.alpha",
				Resources: []string{"connections"},
				Verbs:     []string{"list"},
			},
		}, permissions)
	})

	t.Run("wrapped permissions array", func(t *testing.T) {
		path := writeTestFile(t, `
permissions:
  - namespace: root.beta
    resources: [actors]
    verbs: [get]
`)

		permissions, err := readPermissionsFile(path)
		require.NoError(t, err)
		require.Equal(t, []aschema.Permission{
			{
				Namespace: "root.beta",
				Resources: []string{"actors"},
				Verbs:     []string{"get"},
			},
		}, permissions)
	})

	t.Run("missing permissions returns error", func(t *testing.T) {
		path := writeTestFile(t, `permissions: []`)

		_, err := readPermissionsFile(path)
		require.Error(t, err)
	})
}

func TestResolveBuilderWithGrafanaPreset(t *testing.T) {
	t.Parallel()

	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC))
	secretPath := writeTestFile(t, "test-secret")

	resolver := &Resolver{
		actorId:       "grafana",
		secretKeyPath: secretPath,
		apis:          string(config.ServiceIdApi),
		grafanaPreset: "logs",
	}

	builder, err := resolver.ResolveBuilder()
	require.NoError(t, err)

	token, err := builder.TokenCtx(ctx)
	require.NoError(t, err)

	claims := parseTestToken(t, ctx, token, []byte("test-secret"))
	require.Equal(t, "grafana", claims.Subject)
	require.Equal(t, []string{string(config.ServiceIdApi)}, []string(claims.Audience))
	require.True(t, apctx.GetClock(ctx).Now().Add(grafanaDefaultExpiresIn).Equal(claims.ExpiresAt.Time))
	require.Contains(t, claims.Permissions, aschema.Permission{
		Namespace: "root.**",
		Resources: []string{"request-events"},
		Verbs:     []string{"list"},
	})
}

func TestResolveBuilderNoExpiryOverridesGrafanaDefault(t *testing.T) {
	t.Parallel()

	ctx := apctx.WithFixedClock(context.Background(), time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC))
	secretPath := writeTestFile(t, "test-secret")

	resolver := &Resolver{
		actorId:       "grafana",
		secretKeyPath: secretPath,
		apis:          string(config.ServiceIdApi),
		grafanaPreset: "aggregate",
		noExpiry:      true,
	}

	builder, err := resolver.ResolveBuilder()
	require.NoError(t, err)

	token, err := builder.TokenCtx(ctx)
	require.NoError(t, err)

	claims := parseTestToken(t, ctx, token, []byte("test-secret"))
	require.Nil(t, claims.ExpiresAt)
	require.NotEmpty(t, claims.Permissions)
}

func TestResolveBuilderMergesPermissionsFile(t *testing.T) {
	t.Parallel()

	secretPath := writeTestFile(t, "test-secret")
	permissionsPath := writeTestFile(t, `
permissions:
  - namespace: root.team
    resources: [connections]
    verbs: [list]
`)

	resolver := &Resolver{
		actorId:         "grafana",
		secretKeyPath:   secretPath,
		apis:            string(config.ServiceIdApi),
		grafanaPreset:   "aggregate",
		permissionsFile: permissionsPath,
	}

	permissions, err := resolver.resolveTokenPermissions()
	require.NoError(t, err)

	require.Contains(t, permissions, aschema.Permission{
		Namespace: "root.**",
		Resources: []string{"app-metrics"},
		Verbs:     []string{"schema", "query"},
	})
	require.Contains(t, permissions, aschema.Permission{
		Namespace: "root.team",
		Resources: []string{"connections"},
		Verbs:     []string{"list"},
	})
}

func writeTestFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func parseTestToken(t *testing.T, ctx context.Context, token string, secret []byte) *apjwt.AuthProxyClaims {
	t.Helper()

	claims, err := apjwt.NewJwtTokenParserBuilder().
		WithSharedKey(secret).
		ParseCtx(ctx, token)
	require.NoError(t, err)
	return claims
}
