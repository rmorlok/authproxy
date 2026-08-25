package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

var testConnectorID = apid.MustParse("cxr_testgmail0000001")

const testBaseURL = "http://seed.test"

func demoUserSeed() ActorSeed {
	return ActorSeed{
		ExternalId: "demo-user",
		Namespace:  "root.demo",
		Permissions: []aschema.Permission{
			{
				Namespace: "root.demo",
				Resources: []string{"connectors"},
				Verbs:     []string{"list"},
			},
			{
				Namespace: "root.demo.{{external_id}}",
				Resources: []string{"connections"},
				Verbs:     []string{"create", "list", "get", "update", "disconnect"},
			},
		},
		Labels: map[string]string{"demo": "true", "role": "user"},
	}
}

func TestUpsertNamespaceCreatesMissingNamespace(t *testing.T) {
	seed := NamespaceSeed{Path: "root.demo", Labels: map[string]string{"demo": "true"}}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			w.WriteHeader(http.StatusNotFound)
		case "POST /api/v1/namespaces":
			var req api.CreateNamespaceRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Path, req.Path)
			require.Equal(t, seed.Labels, req.Labels)
			writeJSON(t, w, api.NamespaceJson{Path: seed.Path, Labels: seed.Labels})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedCreated, action)
}

func TestUpsertNamespaceIgnoresSystemManagedLabels(t *testing.T) {
	seed := NamespaceSeed{Path: "root.demo", Labels: map[string]string{"demo": "true"}}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		writeJSON(t, w, api.NamespaceJson{
			Path: seed.Path,
			Labels: map[string]string{
				"demo":           "true",
				"apxy/ns/-/id":   "root.demo",
				"apxy/ns/-/name": "demo",
				"apxy/ns/-/ns":   "root.demo",
			},
		})
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedAlreadyPresent, action)
}

func TestUpsertNamespaceReconcilesUserMetadata(t *testing.T) {
	seed := NamespaceSeed{
		Path:        "root.demo",
		Labels:      map[string]string{"demo": "true"},
		Annotations: map[string]string{"description": "Demo resources"},
	}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			writeJSON(t, w, api.NamespaceJson{
				Path: seed.Path,
				Labels: map[string]string{
					"demo":         "false",
					"apxy/ns/-/id": "root.demo",
				},
				Annotations: map[string]string{"description": "Stale description"},
			})
		case "PATCH /api/v1/namespaces/root.demo":
			var req api.UpdateNamespaceRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Labels, req.Labels)
			require.Equal(t, seed.Annotations, req.Annotations)
			writeJSON(t, w, api.NamespaceJson{
				Path:        seed.Path,
				Labels:      seed.Labels,
				Annotations: seed.Annotations,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

func TestUpsertNamespaceClearsUnconfiguredUserMetadata(t *testing.T) {
	seed := NamespaceSeed{Path: "root.demo"}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			writeJSON(t, w, api.NamespaceJson{
				Path:        seed.Path,
				Labels:      map[string]string{"legacy": "true"},
				Annotations: map[string]string{"legacy": "true"},
			})
		case "PATCH /api/v1/namespaces/root.demo":
			var req api.UpdateNamespaceRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NotNil(t, req.Labels)
			require.Empty(t, req.Labels)
			require.NotNil(t, req.Annotations)
			require.Empty(t, req.Annotations)
			writeJSON(t, w, api.NamespaceJson{Path: seed.Path})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

func TestUpsertActorCreatesThenAppliesPermissions(t *testing.T) {
	seed := demoUserSeed()
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/actors/external-id/demo-user":
			require.Equal(t, seed.Namespace, r.URL.Query().Get("namespace"))
			w.WriteHeader(http.StatusNotFound)
		case "POST /api/v1/actors":
			var req api.CreateActorRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.ExternalId, req.ExternalId)
			require.Equal(t, seed.Namespace, req.Namespace)
			writeJSON(t, w, api.ActorJson{ExternalId: seed.ExternalId, Namespace: seed.Namespace})
		case "PATCH /api/v1/actors/external-id/demo-user":
			require.Equal(t, seed.Namespace, r.URL.Query().Get("namespace"))
			var req api.UpdateActorRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Permissions, req.Permissions)
			require.Equal(t, seed.Labels, req.Labels)
			writeJSON(t, w, api.ActorJson{
				ExternalId:  seed.ExternalId,
				Namespace:   seed.Namespace,
				Permissions: seed.Permissions,
				Labels:      seed.Labels,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertActor(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedCreated, action)
}

func TestUpsertActorVerifiesExistingActorWithoutChangingIt(t *testing.T) {
	seed := demoUserSeed()
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		writeJSON(t, w, api.ActorJson{
			ExternalId:  seed.ExternalId,
			Namespace:   seed.Namespace,
			Permissions: seed.Permissions,
			Labels:      seed.Labels,
		})
	}))

	action, err := upsertActor(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedAlreadyPresent, action)
}

func TestUpsertActorReconcilesDrift(t *testing.T) {
	seed := demoUserSeed()
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(t, w, api.ActorJson{
				ExternalId:  seed.ExternalId,
				Namespace:   seed.Namespace,
				Permissions: aschema.AllPermissions(),
				Labels:      seed.Labels,
			})
		case http.MethodPatch:
			var req api.UpdateActorRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Permissions, req.Permissions)
			writeJSON(t, w, api.ActorJson{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertActor(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

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
			require.Equal(t, seedLabelKey+"=demo-noauth", r.URL.Query().Get("labelSelector"))
			writeJSON(t, w, api.ListConnectorsResponseJson{})
		case "POST /api/v1/connectors":
			var req api.CreateConnectorRequestJson
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, defaultNamespace, req.Namespace)
			require.Equal(t, "Demo NoAuth", req.Definition.DisplayName)
			require.Equal(t, "demo-noauth", req.Labels[seedLabelKey])
			require.Equal(t, "true", req.Labels["demo"])
			writeJSON(t, w, connectorVersion(req.Definition, req.Labels, api.ConnectorVersionStateDraft, 1))
		case "PUT /api/v1/connectors/cxr_testgmail0000001/versions/1/_forceState":
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
		case "PUT /api/v1/connectors/cxr_testgmail0000001/versions/2/_forceState":
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
			var req struct {
				Key                     string `json:"key"`
				Secret                  string `json:"secret"`
				RedirectURI             string `json:"redirect_uri"`
				TokenEndpointAuthMethod string `json:"token_endpoint_auth_method"`
				RequirePKCE             bool   `json:"require_pkce"`
				Scope                   string `json:"scope"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "demo-oauth-simple", req.Key)
			require.Equal(t, "secret", req.Secret)
			require.Equal(t, "https://marketplace.example.test/oauth2/callback", req.RedirectURI)
			require.Equal(t, "client_secret_post", req.TokenEndpointAuthMethod)
			require.True(t, req.RequirePKCE)
			require.Equal(t, "read", req.Scope)
			w.WriteHeader(http.StatusCreated)
		case "POST /test/users":
			var req struct {
				Username    string `json:"username"`
				Password    string `json:"password"`
				DisplayName string `json:"display_name"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "demo-oauth-user@example.test", req.Username)
			require.Equal(t, "demo-password", req.Password)
			require.Equal(t, "Demo OAuth User", req.DisplayName)
			w.WriteHeader(http.StatusCreated)
		case "POST /test/resource-policy":
			var req struct {
				Path          string `json:"path"`
				RequiredScope string `json:"required_scope"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "/test/resource/demo-resources", req.Path)
			require.Equal(t, "read", req.RequiredScope)
			w.WriteHeader(http.StatusNoContent)
		case "POST /test/api-key-resource-policy":
			var req struct {
				Path       string `json:"path"`
				Key        string `json:"key"`
				Placement  string `json:"placement"`
				HeaderName string `json:"header_name"`
				Prefix     string `json:"prefix"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "/test/api-key-resource/demo-api-key", req.Path)
			require.Equal(t, "demo-api-key", req.Key)
			require.Equal(t, "header", req.Placement)
			require.Equal(t, "X-Demo-Key", req.HeaderName)
			require.Equal(t, "Token ", req.Prefix)
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
			RequirePKCE:             true,
			Scope:                   "read",
		}},
		Users: []OAuth2TestProviderUser{{
			Username:    "demo-oauth-user@example.test",
			Password:    "demo-password",
			DisplayName: "Demo OAuth User",
		}},
		ResourcePolicies: []OAuth2ResourcePolicy{{
			Path:          "/test/resource/demo-resources",
			RequiredScope: "read",
		}},
		APIKeyResourcePolicies: []APIKeyResourcePolicy{{
			Path:       "/test/api-key-resource/demo-api-key",
			Key:        "demo-api-key",
			Placement:  "header",
			HeaderName: "X-Demo-Key",
			Prefix:     "Token ",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, seen["POST /test/clients"])
	require.Equal(t, 1, seen["POST /test/users"])
	require.Equal(t, 1, seen["POST /test/resource-policy"])
	require.Equal(t, 1, seen["POST /test/api-key-resource-policy"])
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
oauth2TestProvider:
  baseUrl: http://go-oauth2-server
  clients:
    - key: demo-oauth-simple
      secret: demo-oauth-simple-secret
      redirectUri: https://marketplace.example.test/oauth2/callback
      tokenEndpointAuthMethod: client_secret_post
      scope: read profile resources
  users:
    - username: demo-oauth-user@example.test
      password: demo-password
connectors:
  - key: demo-oauth-tenant
    namespace: root
    definition:
      displayName: Demo OAuth Tenant
      description: Demo OAuth connector with pre-connect config
      labels:
        type: demo-oauth-tenant
      auth:
        type: OAuth2
        clientId: demo-oauth-tenant
        clientSecret: demo-oauth-tenant-secret
        authorization:
          endpoint: https://example.test/oauth2/web/authorize
          queryOverrides:
            tenant: "{{cfg.tenant}}"
        token:
          endpoint: http://go-oauth2-server/v1/oauth/tokens
        scopes:
          - id: read
            reason: Read demo data
      setupFlow:
        preconnect:
          steps:
            - id: tenant
              title: Choose tenant
              jsonSchema:
                type: object
                required:
                  - tenant
                properties:
                  tenant:
                    type: string
              uiSchema:
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

func TestSeedConfigParsesAPIKeyConnector(t *testing.T) {
	data := []byte(`
oauth2TestProvider:
  baseUrl: http://go-oauth2-server
  apiKeyResourcePolicies:
    - path: /test/api-key-resource/demo-api-key
      key: demo-api-key
      placement: bearer
connectors:
  - key: demo-api-key
    namespace: root
    definition:
      displayName: Demo API Key
      description: Demo API key connector
      labels:
        type: demo-api-key
      auth:
        type: api-key
        placement:
          type: bearer
      probes:
        - id: verify-api-key
          proxyHttp:
            method: GET
            url: http://go-oauth2-server/test/api-key-resource/demo-api-key
`)
	var cfg SeedConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.OAuth2TestProvider)
	require.Len(t, cfg.OAuth2TestProvider.APIKeyResourcePolicies, 1)
	require.Len(t, cfg.Connectors, 1)
	require.NoError(t, cfg.Connectors[0].Definition.Validate(&common.ValidationContext{}))
}

func TestDeploymentSeedConfigsUseIsolatedDemoResources(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "..", "deploy", "kustomize", "authproxy-demo", "overlays", "demo", "seed", "seed-config.yaml"),
		filepath.Join("..", "..", "..", "deploy", "kustomize", "authproxy-demo", "overlays", "dev", "seed", "seed-config.yaml"),
	}

	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(filepath.Dir(path))), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			var configMap struct {
				Data map[string]string `yaml:"data"`
			}
			require.NoError(t, yaml.Unmarshal(data, &configMap))

			var cfg SeedConfig
			require.NoError(t, util.DecodeYAMLStrict([]byte(configMap.Data["seed.yaml"]), &cfg))
			require.NotNil(t, cfg.OAuth2TestProvider)
			require.Equal(t, []NamespaceSeed{{Path: "root.demo", Labels: map[string]string{"demo": "true"}}}, cfg.Namespaces)
			require.Len(t, cfg.Actors, 1)
			require.Equal(t, demoUserSeed(), cfg.Actors[0])
			require.Len(t, cfg.Connectors, 5)

			providerClients := make(map[string]OAuth2TestProviderClient, len(cfg.OAuth2TestProvider.Clients))
			for _, client := range cfg.OAuth2TestProvider.Clients {
				providerClients[client.Key] = client
			}
			oauthConnectorCount := 0
			for _, connector := range cfg.Connectors {
				require.Equal(t, "root.demo", connectorNamespace(connector))
				require.NoError(t, connector.Definition.Validate(&common.ValidationContext{}))

				oauthAuth, ok := connector.Definition.Auth.Inner().(*config.AuthOAuth2)
				if !ok {
					continue
				}
				oauthConnectorCount++
				clientID, err := oauthAuth.ClientId.GetValue(context.Background())
				require.NoError(t, err)
				providerClient, ok := providerClients[clientID]
				require.Truef(t, ok, "OAuth connector %q references unseeded provider client %q", connector.Key, clientID)
				require.Equal(t, string(oauthAuth.GetTokenEndpointAuthMethodOrDefault()), providerClient.TokenEndpointAuthMethod)
			}
			require.Equal(t, 3, oauthConnectorCount)
		})
	}
}

func TestComposeSeedConfigUsesIsolatedDemoResources(t *testing.T) {
	path := filepath.Join("..", "..", "shell", "compose", "seed.yaml")
	cfg, err := loadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Namespaces, 1)
	require.Equal(t, "root.demo", cfg.Namespaces[0].Path)
	require.Equal(t, []ActorSeed{demoUserSeed()}, cfg.Actors)
	require.Len(t, cfg.Connectors, 1)
	require.Equal(t, "root.demo", connectorNamespace(cfg.Connectors[0]))
	require.NoError(t, cfg.Connectors[0].Definition.Validate(&common.ValidationContext{}))
}

func mustConnector(t *testing.T, displayName string) config.Connector {
	t.Helper()

	data := []byte(`
displayName: "` + displayName + `"
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
