package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const testBaseURL = "http://seed.test"

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

// The same strict loader used by ap apply validates every deployment fixture.
// This catches config-only fields, omitted namespaces, and invalid definitions
// before the job reaches a cluster.
func TestDemoApplyManifests(t *testing.T) {
	for _, environment := range []string{"demo", "dev", "compose"} {
		t.Run(environment, func(t *testing.T) {
			var providerYAML, resources string
			wantConnectors := 5
			if environment == "compose" {
				data, err := os.ReadFile("../../shell/compose/resources.yaml")
				require.NoError(t, err)
				resources = string(data)
				data, err = os.ReadFile("../../shell/compose/seed.yaml")
				require.NoError(t, err)
				providerYAML = string(data)
				wantConnectors = 1
			} else {
				data, err := os.ReadFile(filepath.Join("../../../deploy/kustomize/authproxy-demo/overlays", environment, "seed/seed-config.yaml"))
				require.NoError(t, err)
				var cm struct {
					Data map[string]string `yaml:"data"`
				}
				require.NoError(t, yaml.Unmarshal(data, &cm))
				resources, providerYAML = cm.Data["resources.yaml"], cm.Data["seed.yaml"]
			}
			var provider SeedConfig
			require.NoError(t, util.DecodeYAMLStrict([]byte(providerYAML), &provider))
			docs, err := apply.Load(context.Background(), apply.Options{
				Filenames: []string{"-"}, Stdin: strings.NewReader(resources), Validation: apply.ValidationStrict,
			})
			require.NoError(t, err)
			require.Len(t, docs, wantConnectors+2)
			counts := map[string]int{}
			for _, doc := range docs {
				counts[string(doc.Kind)]++
				if doc.Kind == "Namespace" {
					require.Equal(t, "root", doc.Metadata.Namespace)
					require.Equal(t, "demo", string(doc.Metadata.Name))
				} else {
					require.Equal(t, "root.demo", doc.Metadata.Namespace)
				}
			}
			require.Equal(t, map[string]int{"Namespace": 1, "Actor": 1, "Connector": wantConnectors}, counts)
		})
	}
}

func TestProviderConfigRejectsAuthProxyResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.yaml")
	require.NoError(t, os.WriteFile(path, []byte("connectors: []\n"), 0600))
	_, err := loadConfig(path)
	require.ErrorContains(t, err, "parse provider seed config")
}

// Server configuration must never synchronize a second copy of the catalog.
// An empty loader is required by the config schema but contains no resources.
func TestDemoServerConfigsHaveNoConnectorDefinitions(t *testing.T) {
	paths := []string{
		"../../shell/compose/authproxy.yaml",
		"../../../deploy/kustomize/authproxy-demo/overlays/demo/authproxy-config.yaml",
		"../../../deploy/kustomize/authproxy-demo/overlays/dev/authproxy-config.yaml",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var cfg map[string]any
			require.NoError(t, yaml.Unmarshal(data, &cfg))
			require.Equal(t, map[string]any{"loadFromList": []any{}}, cfg["connectors"])
			var serverConfig config.Root
			require.NoError(t, util.DecodeYAMLStrict(data, &serverConfig))
			require.NoError(t, serverConfig.Validate())
		})
	}
}
