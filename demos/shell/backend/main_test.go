package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	authjwt "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/stretchr/testify/require"
)

func TestActorExternalID(t *testing.T) {
	t.Run("keeps well-known actor external IDs stable", func(t *testing.T) {
		require.Equal(t, "demo-admin", actorExternalID("demo-admin"))
		require.Equal(t, "demo-user", actorExternalID("demo-user"))
	})

	t.Run("generates a new external ID for every fresh user", func(t *testing.T) {
		first := actorExternalID(freshUserSelection)
		second := actorExternalID(freshUserSelection)

		require.NotEqual(t, first, second)
		for _, externalID := range []string{first, second} {
			suffix, found := strings.CutPrefix(externalID, freshUserSelection+"-")
			require.True(t, found)
			require.NoError(t, uuid.Validate(suffix))
		}
	})
}

func TestSignTokenForFreshUserIncludesActorForJustInTimeProvisioning(t *testing.T) {
	externalID := actorExternalID(freshUserSelection)
	keyDir := filepath.Join("..", "..", "..", "test_data", "admin_user_keys")
	token, err := signTokenForFreshUser(settings{
		jwtPrivateKeyPath: filepath.Join(keyDir, "bobdole"),
		tokenTtl:          time.Minute,
	}, externalID)
	require.NoError(t, err)

	claims, err := authjwt.NewJwtTokenParserBuilder().
		WithPublicKeyPath(filepath.Join(keyDir, "bobdole.pub")).
		Parse(token)
	require.NoError(t, err)
	require.Equal(t, externalID, claims.Subject)
	require.False(t, claims.ActorSigned)
	require.NotNil(t, claims.Actor)
	require.Equal(t, externalID, claims.Actor.ExternalId)
	require.Equal(t, "root", claims.Actor.Namespace)
	require.Equal(t, map[string]string{"demo": "true", "role": "user"}, claims.Actor.Labels)
	require.Len(t, claims.Actor.Permissions, 1)
	require.Equal(t, []string{"*"}, claims.Actor.Permissions[0].Resources)
	require.Equal(t, []string{"*"}, claims.Actor.Permissions[0].Verbs)
}

func TestLoadTelemetryLinksFromGrafanaBaseURL(t *testing.T) {
	t.Setenv("AUTHPROXY_GRAFANA_URL", "https://demo.example.test/grafana/")
	t.Setenv("AUTHPROXY_GRAFANA_APP_METRICS_URL", "")
	t.Setenv("AUTHPROXY_GRAFANA_EXPLORE_URL", "")

	links := loadTelemetryLinks()

	require.Equal(t, []telemetryLink{
		{
			Label:       "Grafana",
			Description: "Open the demo observability workspace and navigate Grafana as you wish.",
			URL:         "https://demo.example.test/grafana",
		},
		{
			Label:       "App metrics",
			Description: "View request, resource, connection, and rate-limit telemetry. Go to the dashboard for the AuthProxy Grafana data source plugin.",
			URL:         "https://demo.example.test/grafana/d/authproxy-app-metrics-demo/authproxy-app-metrics?orgId=1&from=now-1h&to=now",
		},
		{
			Label:       "Explore",
			Description: "Query telemetry directly in Grafana.",
			URL:         "https://demo.example.test/grafana/explore?orgId=1",
		},
	}, links)
}

func TestLoadTelemetryLinksAllowsExplicitURLsWithoutGrafanaBaseURL(t *testing.T) {
	t.Setenv("AUTHPROXY_GRAFANA_URL", "")
	t.Setenv("AUTHPROXY_GRAFANA_APP_METRICS_URL", "https://grafana.example.test/d/app")
	t.Setenv("AUTHPROXY_GRAFANA_EXPLORE_URL", "https://grafana.example.test/explore")

	links := loadTelemetryLinks()

	require.Equal(t, []telemetryLink{
		{
			Label:       "App metrics",
			Description: "View request, resource, connection, and rate-limit telemetry. Go to the dashboard for the AuthProxy Grafana data source plugin.",
			URL:         "https://grafana.example.test/d/app",
		},
		{
			Label:       "Explore",
			Description: "Query telemetry directly in Grafana.",
			URL:         "https://grafana.example.test/explore",
		},
	}, links)
}

func TestLoadOAuthProviderURL(t *testing.T) {
	t.Setenv("AUTHPROXY_GO_OAUTH2_SERVER_URL", " https://oauth2.demo.example.test/ ")

	require.Equal(t, "https://oauth2.demo.example.test", loadOAuthProviderURL())
}

func TestConfigHandlerReturnsTelemetryLinks(t *testing.T) {
	links := []telemetryLink{{
		Label:       "Grafana",
		Description: "Open the demo observability workspace.",
		URL:         "https://demo.example.test/grafana",
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config.json", nil)

	configHandler(settings{
		oauthProviderURL: "https://oauth2.demo.example.test",
		telemetryLinks:   links,
	}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		OAuthProviderURL string          `json:"oauthProviderUrl"`
		TelemetryLinks   []telemetryLink `json:"telemetryLinks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "https://oauth2.demo.example.test", body.OAuthProviderURL)
	require.Equal(t, links, body.TelemetryLinks)
}
