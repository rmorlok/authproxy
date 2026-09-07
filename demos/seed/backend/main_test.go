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
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

var testConnectorID = apid.MustParse("cxr_testgmail0000001")

const testBaseURL = "http://seed.test"

func demoUserSeed() actorschema.Actor {
	actor := actorschema.NewActor()
	actor.Metadata.Name = "demo-user"
	actor.Metadata.Namespace = "root.demo"
	actor.Metadata.Labels = map[string]string{"demo": "true", "role": "user"}
	actor.Spec.ExternalId = "demo-user"
	actor.Spec.Permissions = []aschema.Permission{
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
	}
	return *actor
}

func demoNamespaceSeed(t *testing.T) nschema.Namespace {
	t.Helper()
	ns, err := nschema.NewNamespaceForPath("root.demo")
	require.NoError(t, err)
	ns.Metadata.Labels = map[string]string{"demo": "true"}
	return *ns
}

func TestUpsertNamespaceCreatesMissingNamespace(t *testing.T) {
	seed := demoNamespaceSeed(t)
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			w.WriteHeader(http.StatusNotFound)
		case "POST /api/v1/namespaces":
			var req nschema.Namespace
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed, req)
			writeJSON(t, w, seed)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedCreated, action)
}

func TestUpsertNamespaceIgnoresSystemManagedLabels(t *testing.T) {
	seed := demoNamespaceSeed(t)
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		existing := seed.Clone()
		existing.Metadata.ID = "root.demo"
		existing.Metadata.Labels = map[string]string{
			"demo":           "true",
			"apxy/ns/-/id":   "root.demo",
			"apxy/ns/-/name": "demo",
			"apxy/ns/-/ns":   "root.demo",
		}
		writeJSON(t, w, existing)
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedAlreadyPresent, action)
}

func TestUpsertNamespaceReconcilesUserMetadata(t *testing.T) {
	seed := demoNamespaceSeed(t)
	seed.Metadata.Annotations = map[string]string{"description": "Demo resources"}
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			existing := seed.Clone()
			existing.Metadata.ID = "root.demo"
			existing.Metadata.Labels = map[string]string{
				"demo":         "false",
				"apxy/ns/-/id": "root.demo",
			}
			existing.Metadata.Annotations = map[string]string{"description": "Stale description"}
			writeJSON(t, w, existing)
		case "PATCH /api/v1/namespaces/root.demo":
			var req nschema.NamespacePatch
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Metadata.Labels, *req.Metadata.Labels)
			require.Equal(t, seed.Metadata.Annotations, *req.Metadata.Annotations)
			writeJSON(t, w, seed)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

func TestUpsertNamespaceClearsUnconfiguredUserMetadata(t *testing.T) {
	seed, err := nschema.NewNamespaceForPath("root.demo")
	require.NoError(t, err)
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/namespaces/root.demo":
			existing := seed.Clone()
			existing.Metadata.ID = "root.demo"
			existing.Metadata.Labels = map[string]string{"legacy": "true"}
			existing.Metadata.Annotations = map[string]string{"legacy": "true"}
			writeJSON(t, w, existing)
		case "PATCH /api/v1/namespaces/root.demo":
			var req nschema.NamespacePatch
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NotNil(t, req.Metadata.Labels)
			require.Empty(t, *req.Metadata.Labels)
			require.NotNil(t, req.Metadata.Annotations)
			require.Empty(t, *req.Metadata.Annotations)
			writeJSON(t, w, seed)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertNamespace(client, testBaseURL, *seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

func TestUpsertActorCreatesMissingActor(t *testing.T) {
	seed := demoUserSeed()
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/actors/external-id/demo-user":
			require.Equal(t, seed.Metadata.Namespace, r.URL.Query().Get("namespace"))
			w.WriteHeader(http.StatusNotFound)
		case "POST /api/v1/actors":
			var req actorschema.Actor
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed, req)
			writeJSON(t, w, seed)
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
		existing := seed.Clone()
		existing.Metadata.ID = "act_demouser000001"
		writeJSON(t, w, existing)
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
			existing := seed.Clone()
			existing.Metadata.ID = "act_demouser000001"
			existing.Spec.Permissions = aschema.AllPermissions()
			writeJSON(t, w, existing)
		case http.MethodPatch:
			require.Equal(t, "/api/v1/actors/act_demouser000001", r.URL.Path)
			var req actorschema.ActorPatch
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, seed.Spec.Permissions, *req.Spec.Permissions)
			writeJSON(t, w, seed)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertActor(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, seedUpdated, action)
}

func TestUpsertConnectorCreatesAndPublishesMissingSeed(t *testing.T) {
	seed := seedConnector(t, "demo-noauth", "Demo NoAuth")

	forcedPrimary := false
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			require.Equal(t, "root", r.URL.Query().Get("namespace"))
			require.Equal(t, "demo-noauth", r.URL.Query().Get("name"))
			writeJSON(t, w, api.NewListConnectorsResponseJson(nil, ""))
		case "POST /api/v1/connectors":
			var req cschema.Connector
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "root", req.Metadata.Namespace)
			require.Equal(t, "demo-noauth", string(req.Metadata.Name))
			require.Equal(t, "Demo NoAuth", req.Spec.Definition.DisplayName)
			require.Equal(t, "true", req.Metadata.Labels["demo"])
			writeJSON(t, w, connectorVersion(req.Spec.Definition, req.Metadata.Labels, cschema.ConnectorReleaseStateDraft, 1))
		case "PUT /api/v1/connectors/cxr_testgmail0000001/generations/1/_forceState":
			forcedPrimary = true
			writeJSON(t, w, connectorVersion(seed.Spec.Definition, seed.Metadata.Labels, cschema.ConnectorReleaseStatePrimary, 1))
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
	seed := seedConnector(t, "demo-noauth", "Demo NoAuth")

	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			writeJSON(t, w, api.NewListConnectorsResponseJson(
				[]cschema.Connector{
					connectorSummary(
						seed,
						cschema.ConnectorReleaseStatePrimary,
						1, // version
					),
				},
				"", // continueToken
			))
		case "GET /api/v1/connectors/cxr_testgmail0000001/generations/1":
			writeJSON(t, w, connectorVersion(seed.Spec.Definition, seed.Metadata.Labels, cschema.ConnectorReleaseStatePrimary, 1))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))

	action, err := upsertConnector(client, testBaseURL, seed)
	require.NoError(t, err)
	require.Equal(t, connectorAlreadyPresent, action)
}

func TestUpsertConnectorPublishesNewVersionWhenDefinitionChanges(t *testing.T) {
	seed := seedConnector(t, "demo-noauth", "New Demo NoAuth")
	oldDefinition := mustConnector(t, "Old Demo NoAuth")
	forcedPrimary := false

	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/connectors":
			writeJSON(t, w, api.NewListConnectorsResponseJson(
				[]cschema.Connector{connectorSummary(seed, cschema.ConnectorReleaseStatePrimary, 1)},
				"",
			))
		case "GET /api/v1/connectors/cxr_testgmail0000001/generations/1":
			writeJSON(t, w, connectorVersion(oldDefinition, seed.Metadata.Labels, cschema.ConnectorReleaseStatePrimary, 1))
		case "POST /api/v1/connectors/cxr_testgmail0000001/generations":
			var req cschema.Connector
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "New Demo NoAuth", req.Spec.Definition.DisplayName)
			require.NotNil(t, req.Metadata.Labels)
			writeJSON(t, w, connectorVersion(req.Spec.Definition, req.Metadata.Labels, cschema.ConnectorReleaseStateDraft, 2))
		case "PUT /api/v1/connectors/cxr_testgmail0000001/generations/2/_forceState":
			forcedPrimary = true
			writeJSON(t, w, connectorVersion(seed.Spec.Definition, seed.Metadata.Labels, cschema.ConnectorReleaseStatePrimary, 2))
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

func TestDeploymentSeedConfigsContainCanonicalResources(t *testing.T) {
	for _, overlay := range []string{"demo", "dev"} {
		t.Run(overlay, func(t *testing.T) {
			manifestData, err := os.ReadFile(filepath.Join(
				"..", "..", "..", "deploy", "kustomize", "authproxy-demo", "overlays", overlay, "seed", "seed-config.yaml",
			))
			require.NoError(t, err)

			var manifest struct {
				APIVersion string `yaml:"apiVersion"`
				Kind       string `yaml:"kind"`
				Metadata   struct {
					Name string `yaml:"name"`
				} `yaml:"metadata"`
				Data map[string]string `yaml:"data"`
			}
			require.NoError(t, util.DecodeYAMLStrict(manifestData, &manifest))
			require.Equal(t, "v1", manifest.APIVersion)
			require.Equal(t, "ConfigMap", manifest.Kind)

			seedPath := filepath.Join(t.TempDir(), "seed.yaml")
			require.NoError(t, os.WriteFile(seedPath, []byte(manifest.Data["seed.yaml"]), 0o600))
			cfg, err := loadConfig(seedPath)
			require.NoError(t, err)
			require.NotEmpty(t, cfg.Actors)
			require.NotEmpty(t, cfg.Connectors)
			for _, actor := range cfg.Actors {
				require.Equal(t, "authproxy.net/v1alpha1", string(actor.APIVersion))
				require.Equal(t, "Actor", string(actor.Kind))
			}
			for _, connector := range cfg.Connectors {
				require.Equal(t, "authproxy.net/v1alpha1", string(connector.APIVersion))
				require.Equal(t, "Connector", string(connector.Kind))
			}
		})
	}
}

func TestLoadConfigRejectsLegacyFlatResources(t *testing.T) {
	seedPath := filepath.Join(t.TempDir(), "seed.yaml")
	require.NoError(t, os.WriteFile(seedPath, []byte(`
actors:
  - externalId: demo-admin
    namespace: root
connectors:
  - key: demo-noauth
    namespace: root
    definition:
      displayName: Demo NoAuth
      auth:
        type: no-auth
`), 0o600))

	_, err := loadConfig(seedPath)
	require.ErrorContains(t, err, "parse seed config")
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
  - apiVersion: authproxy.net/v1alpha1
    kind: Connector
    metadata:
      name: demo-oauth-tenant
      namespace: root
      labels:
        type: demo-oauth-tenant
    spec:
      release:
        desiredState: primary
      definition:
        displayName: Demo OAuth Tenant
        description: Demo OAuth connector with pre-connect config
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
	require.NoError(t, cfg.Connectors[0].Spec.Definition.Validate(&common.ValidationContext{}))
	require.True(t, cfg.Connectors[0].Spec.Definition.SetupFlow.HasPreconnect())
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
  - apiVersion: authproxy.net/v1alpha1
    kind: Connector
    metadata:
      name: demo-api-key
      namespace: root
      labels:
        type: demo-api-key
    spec:
      release:
        desiredState: primary
      definition:
        displayName: Demo API Key
        description: Demo API key connector
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
	require.NoError(t, cfg.Connectors[0].Spec.Definition.Validate(&common.ValidationContext{}))
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
			require.Len(t, cfg.Namespaces, 1)
			require.Equal(t, demoNamespaceSeed(t), cfg.Namespaces[0])
			require.Len(t, cfg.Actors, 1)
			require.Equal(t, demoUserSeed(), cfg.Actors[0])
			require.Len(t, cfg.Connectors, 5)

			providerClients := make(map[string]OAuth2TestProviderClient, len(cfg.OAuth2TestProvider.Clients))
			for _, client := range cfg.OAuth2TestProvider.Clients {
				providerClients[client.Key] = client
			}
			oauthConnectorCount := 0
			for _, connector := range cfg.Connectors {
				require.Equal(t, "root.demo", connector.Metadata.Namespace)
				require.NotEmpty(t, connector.Metadata.Labels["demo.authproxy.net/seed-key"])
				require.NoError(t, connector.Spec.Definition.Validate(&common.ValidationContext{}))

				oauthAuth, ok := connector.Spec.Definition.Auth.Inner().(*config.AuthOAuth2)
				if !ok {
					continue
				}
				oauthConnectorCount++
				clientID, err := oauthAuth.ClientId.GetValue(context.Background())
				require.NoError(t, err)
				providerClient, ok := providerClients[clientID]
				require.Truef(t, ok, "OAuth connector %q references unseeded provider client %q", connector.Metadata.Name, clientID)
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
	require.Equal(t, demoNamespaceSeed(t), cfg.Namespaces[0])
	require.Equal(t, []actorschema.Actor{demoUserSeed()}, cfg.Actors)
	require.Len(t, cfg.Connectors, 1)
	require.Equal(t, "root.demo", cfg.Connectors[0].Metadata.Namespace)
	require.NoError(t, cfg.Connectors[0].Spec.Definition.Validate(&common.ValidationContext{}))
}

func mustConnector(t *testing.T, displayName string) config.ConnectorDefinition {
	t.Helper()

	data := []byte(`
displayName: "` + displayName + `"
description: "Seeded test connector"
labels:
  type: demo-noauth
auth:
  type: no-auth
`)
	var connector config.ConnectorDefinition
	require.NoError(t, yaml.Unmarshal(data, &connector))
	return connector
}

func seedConnector(t *testing.T, name, displayName string) cschema.Connector {
	t.Helper()
	resource := cschema.NewConnector()
	resource.Metadata.Name = common.ResourceName(name)
	resource.Metadata.Namespace = "root"
	resource.Metadata.Labels = map[string]string{"demo": "true"}
	resource.Spec.Release.DesiredState = cschema.ConnectorReleaseStatePrimary
	resource.Spec.Definition = mustConnector(t, displayName)
	return *resource
}

func connectorSummary(seed cschema.Connector, state cschema.ConnectorReleaseState, version uint64) cschema.Connector {
	return connectorVersion(seed.Spec.Definition, seed.Metadata.Labels, state, version)
}

func connectorVersion(def config.ConnectorDefinition, labels map[string]string, state cschema.ConnectorReleaseState, version uint64) cschema.Connector {
	resource := cschema.NewConnector()
	resource.Metadata.ID = testConnectorID.String()
	resource.Metadata.Name = "demo-noauth"
	resource.Metadata.Namespace = "root"
	resource.Metadata.Generation = version
	resource.Metadata.Labels = labels
	resource.Spec.Release.DesiredState = cschema.DesiredReleaseStateForObserved(state)
	resource.Spec.Definition = def
	resource.Status = &cschema.ConnectorStatus{
		Release: cschema.ConnectorReleaseStatus{State: state},
	}
	return *resource
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
