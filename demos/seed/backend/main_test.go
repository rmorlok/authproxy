package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

var testConnectorID = apid.MustParse("cxr_testgmail0000001")

const testBaseURL = "http://seed.test"

func TestUpsertConnectorCreatesAndPublishesMissingSeed(t *testing.T) {
	seed := ConnectorSeed{
		Key:        "demo-noauth",
		Definition: mustConnector(t, "Demo NoAuth"),
		Labels: map[string]string{
			"demo": "true",
		},
	}

	forcedPrimary := false
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			require.Equal(t, defaultNamespace, r.URL.Query().Get("namespace"))
			require.Equal(t, seedLabelKey+"=demo-noauth", r.URL.Query().Get("label_selector"))
			writeJSON(t, w, api.ListConnectorsResponseJson{})
		case "POST /api/v1/connectors":
			var req api.CreateConnectorRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, defaultNamespace, req.Namespace)
			require.Equal(t, "Demo NoAuth", req.Definition.DisplayName)
			require.Equal(t, "demo-noauth", req.Labels[seedLabelKey])
			require.Equal(t, "true", req.Labels["demo"])
			writeJSON(t, w, connectorVersion(req.Definition, req.Labels, api.ConnectorVersionStateDraft, 1))
		case "PUT /api/v1/connectors/cxr_testgmail0000001/versions/1/_force_state":
			forcedPrimary = true
			writeJSON(t, w, connectorVersion(seed.Definition, connectorLabels(seed), api.ConnectorVersionStatePrimary, 1))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertConnector(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, connectorCreated, action)
	require.True(t, forcedPrimary)
}

func TestUpsertConnectorSkipsMatchingPrimarySeed(t *testing.T) {
	seed := ConnectorSeed{
		Key:        "demo-noauth",
		Namespace:  "root",
		Definition: mustConnector(t, "Demo NoAuth"),
		Labels: map[string]string{
			"demo": "true",
		},
	}

	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			writeJSON(t, w, api.ListConnectorsResponseJson{
				Items: []api.ConnectorJson{connectorSummary(seed, api.ConnectorVersionStatePrimary, 1)},
			})
		case "GET /api/v1/connectors/cxr_testgmail0000001/versions/1":
			writeJSON(t, w, connectorVersion(seed.Definition, connectorLabels(seed), api.ConnectorVersionStatePrimary, 1))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertConnector(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, connectorAlreadyPresent, action)
}

func TestUpsertConnectorPublishesNewVersionWhenDefinitionChanges(t *testing.T) {
	seed := ConnectorSeed{
		Key:        "demo-noauth",
		Namespace:  "root",
		Definition: mustConnector(t, "New Demo NoAuth"),
	}
	oldDefinition := mustConnector(t, "Old Demo NoAuth")
	forcedPrimary := false

	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			writeJSON(t, w, api.ListConnectorsResponseJson{
				Items: []api.ConnectorJson{connectorSummary(seed, api.ConnectorVersionStatePrimary, 1)},
			})
		case "GET /api/v1/connectors/cxr_testgmail0000001/versions/1":
			writeJSON(t, w, connectorVersion(oldDefinition, connectorLabels(seed), api.ConnectorVersionStatePrimary, 1))
		case "POST /api/v1/connectors/cxr_testgmail0000001/versions":
			var req api.CreateConnectorVersionRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NotNil(t, req.Definition)
			require.Equal(t, "New Demo NoAuth", req.Definition.DisplayName)
			require.NotNil(t, req.Labels)
			require.Equal(t, "demo-noauth", (*req.Labels)[seedLabelKey])
			writeJSON(t, w, connectorVersion(*req.Definition, *req.Labels, api.ConnectorVersionStateDraft, 2))
		case "PUT /api/v1/connectors/cxr_testgmail0000001/versions/2/_force_state":
			forcedPrimary = true
			writeJSON(t, w, connectorVersion(seed.Definition, connectorLabels(seed), api.ConnectorVersionStatePrimary, 2))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertConnector(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, connectorUpdated, action)
	require.True(t, forcedPrimary)
}

func mustConnector(t *testing.T, displayName string) config.Connector {
	t.Helper()

	data := []byte(`
display_name: "` + displayName + `"
description: "Seeded test connector"
labels:
  type: demo-noauth
auth:
  type: no-auth
`)
	var connector config.Connector
	require.NoError(t, yaml.Unmarshal(data, &connector))
	return connector
}

func connectorSummary(seed ConnectorSeed, state api.ConnectorVersionState, version uint64) api.ConnectorJson {
	return api.ConnectorJson{
		Id:          testConnectorID,
		Version:     version,
		Namespace:   connectorNamespace(seed),
		State:       state,
		DisplayName: seed.Definition.DisplayName,
		Description: seed.Definition.Description,
		Labels:      connectorLabels(seed),
	}
}

func connectorVersion(def config.Connector, labels map[string]string, state api.ConnectorVersionState, version uint64) api.ConnectorVersionJson {
	namespace := defaultNamespace
	def.Id = testConnectorID
	def.Version = version
	def.Namespace = &namespace
	def.State = string(state)
	return api.ConnectorVersionJson{
		Id:         testConnectorID,
		Version:    version,
		Namespace:  namespace,
		State:      state,
		Definition: def,
		Labels:     labels,
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestClient(handler http.Handler) *resty.Client {
	return resty.New().SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	}))
}
