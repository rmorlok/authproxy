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
	"github.com/rmorlok/authproxy/internal/schema/common"
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

func TestSeedOAuth2TestProviderSeedsClientsUsersAndPolicies(t *testing.T) {
	seen := map[string]int{}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path]++
		switch r.Method + " " + r.URL.Path {
		case "POST /test/clients":
			var req OAuth2TestProviderClient
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "demo-oauth-simple", req.Key)
			require.Equal(t, "https://marketplace.example.test/oauth2/callback", req.RedirectURI)
			w.WriteHeader(http.StatusCreated)
		case "POST /test/users":
			var req OAuth2TestProviderUser
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "demo-oauth-user@example.test", req.Username)
			w.WriteHeader(http.StatusCreated)
		case "POST /test/resource-policy":
			var req OAuth2ResourcePolicy
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "/test/resource/demo-resources", req.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	err := seedOAuth2TestProvider(client, OAuth2TestProviderSeed{
		BaseUrl: testBaseURL,
		Clients: []OAuth2TestProviderClient{{
			Key:                     "demo-oauth-simple",
			Secret:                  "secret",
			RedirectURI:             "https://marketplace.example.test/oauth2/callback",
			TokenEndpointAuthMethod: "client_secret_post",
			Scope:                   "read",
		}},
		Users: []OAuth2TestProviderUser{{
			Username: "demo-oauth-user@example.test",
			Password: "demo-password",
		}},
		ResourcePolicies: []OAuth2ResourcePolicy{{
			Path:          "/test/resource/demo-resources",
			RequiredScope: "read",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, seen["POST /test/clients"])
	require.Equal(t, 1, seen["POST /test/users"])
	require.Equal(t, 1, seen["POST /test/resource-policy"])
}

func TestPostOAuth2TestProviderTreatsDuplicateAsAlreadyPresent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "conflict", status: http.StatusConflict},
		{name: "bad request already exists", status: http.StatusBadRequest, body: `{"error":"client already exists"}`},
		{name: "bad request client id taken", status: http.StatusBadRequest, body: `{"error":"Client ID taken"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))

			action, err := postOAuth2TestProvider(client, testBaseURL, "/test/clients", OAuth2TestProviderClient{Key: "demo"})
			require.NoError(t, err)
			require.Equal(t, seedAlreadyPresent, action)
		})
	}
}

func TestSeedConfigParsesOAuthConnectorSetupVariants(t *testing.T) {
	data := []byte(`
oauth2_test_provider:
  base_url: http://go-oauth2-server
  clients:
    - key: demo-oauth-simple
      secret: demo-oauth-simple-secret
      redirect_uri: https://marketplace.example.test/oauth2/callback
      token_endpoint_auth_method: client_secret_post
      scope: read profile resources
  users:
    - username: demo-oauth-user@example.test
      password: demo-password
connectors:
  - key: demo-oauth-tenant
    namespace: root
    definition:
      display_name: Demo OAuth Tenant
      description: Demo OAuth connector with pre-connect config
      labels:
        type: demo-oauth-tenant
      auth:
        type: OAuth2
        client_id: demo-oauth-tenant
        client_secret: demo-oauth-tenant-secret
        authorization:
          endpoint: https://example.test/oauth2/web/authorize
          query_overrides:
            tenant: "{{cfg.tenant}}"
        token:
          endpoint: http://go-oauth2-server/v1/oauth/tokens
        scopes:
          - id: read
            reason: Read demo data
      setup_flow:
        preconnect:
          steps:
            - id: tenant
              title: Choose tenant
              json_schema:
                type: object
                required:
                  - tenant
                properties:
                  tenant:
                    type: string
              ui_schema:
                type: VerticalLayout
                elements:
                  - type: Control
                    scope: "#/properties/tenant"
`)
	var cfg SeedConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.OAuth2TestProvider)
	require.Len(t, cfg.OAuth2TestProvider.Clients, 1)
	require.Len(t, cfg.Connectors, 1)
	require.NoError(t, cfg.Connectors[0].Definition.Validate(&common.ValidationContext{}))
	require.True(t, cfg.Connectors[0].Definition.SetupFlow.HasPreconnect())
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
