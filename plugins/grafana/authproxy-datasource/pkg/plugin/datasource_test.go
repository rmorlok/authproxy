package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

func TestCheckHealthCallsMetricsSchema(t *testing.T) {
	server := newAuthProxyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/metrics/schema", r.URL.Path)
		require.Equal(t, "Bearer test-jwt", r.Header.Get("Authorization"))
		writeJSON(t, w, map[string]any{"metrics": []any{}})
	})
	defer server.Close()

	client, err := newAuthProxyClient(server.URL, "test-jwt", server.Client())
	require.NoError(t, err)

	ds := &Datasource{client: client}
	resp, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	require.NoError(t, err)
	require.Equal(t, backend.HealthStatusOk, resp.Status)
}

func TestQueryDataMetrics(t *testing.T) {
	start := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	server := newAuthProxyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/metrics/query", r.URL.Path)

		var body metricsQueryRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, start, body.Range.Start)
		require.Equal(t, end, body.Range.End)
		require.Equal(t, "15m0s", body.Range.Step)
		require.NotNil(t, body.Namespace)
		require.Equal(t, "root/demo", *body.Namespace)
		require.NotNil(t, body.LabelSelector)
		require.Equal(t, "env=demo", *body.LabelSelector)
		require.Equal(t, []metricsQueryRef{{
			RefID:       "A",
			Metric:      "requests.count",
			Aggregation: "sum",
			GroupBy:     []string{"connector_id"},
		}}, body.Queries)

		writeJSON(t, w, metricsQueryResponse{Series: []metricsSeries{{
			RefID:       "A",
			Metric:      "requests.count",
			Aggregation: "sum",
			Labels:      map[string]string{"connector_id": "conn_slack"},
			Points: []metricsPoint{
				{Timestamp: start, Value: 3},
				{Timestamp: start.Add(15 * time.Minute), Value: 7},
			},
		}}})
	})
	defer server.Close()

	resp := queryTestDatasource(t, server).query(context.Background(), backend.DataQuery{
		RefID:     "A",
		TimeRange: backend.TimeRange{From: start, To: end},
		Interval:  15 * time.Minute,
		JSON: mustJSON(t, queryModel{
			QueryType:     queryTypeMetrics,
			Metric:        "requests.count",
			Aggregation:   "sum",
			GroupBy:       []string{"connector_id"},
			Namespace:     "root/demo",
			LabelSelector: "env=demo",
		}),
	})

	require.NoError(t, resp.Error)
	require.Len(t, resp.Frames, 1)
	require.Equal(t, "A", resp.Frames[0].Name)
	require.Equal(t, "time", resp.Frames[0].Fields[0].Name)
	require.Equal(t, "requests.count.sum", resp.Frames[0].Fields[1].Name)
	require.Equal(t, "conn_slack", string(resp.Frames[0].Fields[1].Labels["connector_id"]))
	require.Equal(t, float64(7), resp.Frames[0].Fields[1].At(1))
}

func TestQueryDataRequestEvents(t *testing.T) {
	ts := time.Date(2026, 5, 30, 10, 15, 0, 0, time.UTC)
	server := newAuthProxyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/metrics/request-events", r.URL.Path)
		require.Equal(t, "root/demo", r.URL.Query().Get("namespace"))
		require.Equal(t, "5xx", r.URL.Query().Get("status_code_range"))
		require.Equal(t, "env=demo", r.URL.Query().Get("label_selector"))
		require.Equal(t, "500", r.URL.Query().Get("limit"))

		writeJSON(t, w, listResponse[requestEvent]{Items: []requestEvent{{
			Timestamp:           ts,
			Namespace:           "root/demo",
			RequestID:           "req_123",
			CorrelationID:       "corr_123",
			Method:              http.MethodGet,
			Path:                "/v1/models",
			ResponseStatusCode:  503,
			MillisecondDuration: 42,
			ConnectionID:        "cxn_123",
			ConnectorID:         "conn_openai",
			ResponseSource:      "upstream",
			RateLimitID:         "rl_123",
			Labels:              map[string]string{"env": "demo", "team": "platform"},
		}}})
	})
	defer server.Close()

	resp := queryTestDatasource(t, server).query(context.Background(), backend.DataQuery{
		RefID: "B",
		JSON: mustJSON(t, queryModel{
			QueryType: queryTypeRequestEvents,
			RequestFilters: requestEventFilters{
				Namespace:       "root/demo",
				StatusCodeRange: "5xx",
				LabelSelector:   "env=demo",
			},
		}),
	})

	require.NoError(t, resp.Error)
	require.Len(t, resp.Frames, 1)
	frame := resp.Frames[0]
	require.Equal(t, "request_events", frame.Name)
	require.Len(t, frame.Fields, 13)
	require.Equal(t, "req_123", frame.Fields[2].At(0))
	require.Equal(t, int64(503), frame.Fields[6].At(0))
	require.Equal(t, `{"env":"demo","team":"platform"}`, frame.Fields[12].At(0))
}

func TestQueryDataVariables(t *testing.T) {
	tests := []struct {
		name          string
		variable      variableQueryOptions
		wantPath      string
		wantConnector string
		response      listResponse[namedResource]
		wantText      string
		wantValue     string
	}{
		{
			name:     "namespaces",
			variable: variableQueryOptions{Type: "namespaces", LabelSelector: "env=demo"},
			wantPath: "/api/v1/namespaces",
			response: listResponse[namedResource]{Items: []namedResource{{Path: "root/demo", DisplayName: "Demo"}}},
			wantText: "Demo", wantValue: "root/demo",
		},
		{
			name:     "connectors",
			variable: variableQueryOptions{Type: "connectors", Namespace: "root/demo"},
			wantPath: "/api/v1/connectors",
			response: listResponse[namedResource]{Items: []namedResource{{ID: "conn_slack", Name: "slack"}}},
			wantText: "slack", wantValue: "conn_slack",
		},
		{
			name:          "connections",
			variable:      variableQueryOptions{Type: "connections", ConnectorID: "conn_slack"},
			wantPath:      "/api/v1/connections",
			wantConnector: "conn_slack",
			response:      listResponse[namedResource]{Items: []namedResource{{ID: "cxn_123", DisplayName: "Slack production"}}},
			wantText:      "Slack production", wantValue: "cxn_123",
		},
		{
			name:     "actors",
			variable: variableQueryOptions{Type: "actors"},
			wantPath: "/api/v1/actors",
			response: listResponse[namedResource]{Items: []namedResource{{ID: "act_123", ExternalID: "svc-demo"}}},
			wantText: "svc-demo", wantValue: "act_123",
		},
		{
			name:     "rate limits",
			variable: variableQueryOptions{Type: "rate_limits"},
			wantPath: "/api/v1/rate-limits",
			response: listResponse[namedResource]{Items: []namedResource{{ID: "rl_123", Name: "global-openai"}}},
			wantText: "global-openai", wantValue: "rl_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAuthProxyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, tt.wantPath, r.URL.Path)
				require.Equal(t, "500", r.URL.Query().Get("limit"))
				require.Equal(t, tt.wantConnector, r.URL.Query().Get("connector_id"))
				writeJSON(t, w, tt.response)
			})
			defer server.Close()

			resp := queryTestDatasource(t, server).query(context.Background(), backend.DataQuery{
				RefID: "var",
				JSON:  mustJSON(t, queryModel{QueryType: queryTypeVariable, Variable: tt.variable}),
			})

			require.NoError(t, resp.Error)
			require.Len(t, resp.Frames, 1)
			require.Equal(t, tt.wantText, resp.Frames[0].Fields[0].At(0))
			require.Equal(t, tt.wantValue, resp.Frames[0].Fields[1].At(0))
		})
	}
}

func TestNewDatasourceRequiresConfiguration(t *testing.T) {
	_, err := NewDatasource(context.Background(), backend.DataSourceInstanceSettings{})
	require.ErrorContains(t, err, "baseUrl is required")

	_, err = NewDatasource(context.Background(), backend.DataSourceInstanceSettings{
		JSONData: mustJSON(t, datasourceSettings{BaseURL: "http://authproxy.example"}),
	})
	require.ErrorContains(t, err, "jwt secure setting is required")
}

func queryTestDatasource(t *testing.T, server *httptest.Server) *Datasource {
	t.Helper()
	client, err := newAuthProxyClient(server.URL, "test-jwt", server.Client())
	require.NoError(t, err)
	return &Datasource{client: client}
}

func newAuthProxyTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-jwt", r.Header.Get("Authorization"))
		handler(w, r)
	}))
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}
