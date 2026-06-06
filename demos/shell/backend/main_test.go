package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadTelemetryLinksFromGrafanaBaseURL(t *testing.T) {
	t.Setenv("AUTHPROXY_GRAFANA_URL", "https://demo.example.test/grafana/")
	t.Setenv("AUTHPROXY_GRAFANA_APP_METRICS_URL", "")
	t.Setenv("AUTHPROXY_GRAFANA_EXPLORE_URL", "")

	links := loadTelemetryLinks()

	require.Equal(t, []telemetryLink{
		{
			Label:       "Grafana",
			Description: "Open the demo observability workspace.",
			URL:         "https://demo.example.test/grafana",
		},
		{
			Label:       "App metrics",
			Description: "View request, resource, connection, and rate-limit telemetry.",
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
			Description: "View request, resource, connection, and rate-limit telemetry.",
			URL:         "https://grafana.example.test/d/app",
		},
		{
			Label:       "Explore",
			Description: "Query telemetry directly in Grafana.",
			URL:         "https://grafana.example.test/explore",
		},
	}, links)
}

func TestConfigHandlerReturnsTelemetryLinks(t *testing.T) {
	links := []telemetryLink{{
		Label:       "Grafana",
		Description: "Open the demo observability workspace.",
		URL:         "https://demo.example.test/grafana",
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config.json", nil)

	configHandler(settings{telemetryLinks: links}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		TelemetryLinks []telemetryLink `json:"telemetryLinks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, links, body.TelemetryLinks)
}
