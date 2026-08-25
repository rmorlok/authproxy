//go:build smoke

package smoke

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	smokeBaseURL   = flag.String("base-url", os.Getenv("SMOKE_BASE_URL"), "base AuthProxy demo URL, e.g. https://demo.authproxy.net")
	smokeGlobalKey = flag.String("global-key", os.Getenv("SMOKE_GLOBAL_KEY"), "AuthProxy global key, or a path to it")
)

const smokeConnectorNamespace = "root.smoke"
const demoConnectorNamespace = "root.demo"

func newSmokeActorExternalID(t *testing.T, prefix string) string {
	t.Helper()

	var entropy [4]byte
	_, err := rand.Read(entropy[:])
	require.NoError(t, err)
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(entropy[:]))
}

func smokeAdminPermissions(adminExternalID, userExternalID, connectionNamespace string) []aschema.Permission {
	return []aschema.Permission{
		{
			Namespace:   config.RootNamespace,
			Resources:   []string{"actors"},
			ResourceIds: []string{adminExternalID},
			Verbs:       []string{"get", "delete"},
		},
		{
			Namespace: demoConnectorNamespace,
			Resources: []string{"connectors"},
			Verbs:     []string{"list", "list/versions"},
		},
		{
			Namespace: smokeConnectorNamespace,
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "create"},
		},
		{
			Namespace: connectionNamespace,
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "create"},
		},
		{
			Namespace:   smokeConnectorNamespace,
			Resources:   []string{"actors"},
			ResourceIds: []string{userExternalID},
			Verbs:       []string{"get", "delete"},
		},
		{
			Namespace: smokeConnectorNamespace,
			Resources: []string{"connectors"},
			Verbs:     []string{"create", "force_state"},
		},
	}
}

func smokeUserPermissions(connectionNamespace string) []aschema.Permission {
	return []aschema.Permission{
		{
			Namespace: demoConnectorNamespace,
			Resources: []string{"connectors"},
			Verbs:     []string{"list"},
		},
		{
			Namespace: smokeConnectorNamespace,
			Resources: []string{"connectors"},
			Verbs:     []string{"list"},
		},
		{
			Namespace: connectionNamespace,
			Resources: []string{"connections"},
			Verbs:     []string{"create", "get", "proxy"},
		},
	}
}

func newRemoteSmokeRig(t *testing.T) *helpers.RemoteAuthProxy {
	t.Helper()

	adminExternalID := newSmokeActorExternalID(t, "smoke-admin")
	userExternalID := newSmokeActorExternalID(t, "smoke-user")
	connectionNamespace := smokeConnectorNamespace + "." + userExternalID
	adminPermissions := smokeAdminPermissions(adminExternalID, userExternalID, connectionNamespace)
	userPermissions := smokeUserPermissions(connectionNamespace)
	rig := helpers.NewRemoteAuthProxy(t, helpers.RemoteAuthProxyOptions{
		BaseURL:               *smokeBaseURL,
		GlobalKey:             *smokeGlobalKey,
		AdminActorExternalID:  adminExternalID,
		AdminActorNamespace:   config.RootNamespace,
		AdminActorPermissions: adminPermissions,
		UserActorExternalID:   userExternalID,
		UserActorNamespace:    smokeConnectorNamespace,
		UserActorPermissions:  userPermissions,
		ConnectorNamespace:    smokeConnectorNamespace,
		ConnectionNamespace:   connectionNamespace,
	})
	rig.EnsureNamespace(t, smokeConnectorNamespace)
	adminActor := rig.GetActorByExternalID(t, config.RootNamespace, adminExternalID)
	require.Equal(t, config.RootNamespace, adminActor.Namespace)
	require.Equal(t, adminExternalID, adminActor.ExternalId)
	require.Equal(t, adminPermissions, adminActor.Permissions)
	t.Cleanup(func() {
		rig.DeleteActorByExternalIDAsAdmin(t, config.RootNamespace, adminExternalID)
	})

	rig.EnsureNamespace(t, connectionNamespace)
	rig.ProvisionUserFromJWT(t)

	actor := rig.GetActorByExternalID(t, smokeConnectorNamespace, userExternalID)
	require.Equal(t, smokeConnectorNamespace, actor.Namespace)
	require.Equal(t, userExternalID, actor.ExternalId)
	require.Equal(t, userPermissions, actor.Permissions)
	t.Cleanup(func() {
		rig.DeleteActorByExternalIDAsAdmin(t, smokeConnectorNamespace, userExternalID)
	})
	return rig
}

func cloneSeededConnectorsIntoSmokeNamespace(
	t *testing.T,
	rig *helpers.RemoteAuthProxy,
	provider *helpers.OAuth2TestProvider,
	oauthClientID string,
	oauthClientSecret string,
) string {
	t.Helper()

	sources := rig.ListConnectorsAsAdmin(t, demoConnectorNamespace, "demo=true")
	require.NotEmpty(t, sources, "the demo seed job did not create any root.demo connectors")
	suffix := time.Now().UnixNano()
	runID := fmt.Sprintf("%d", suffix)
	const runLabel = "smoke.authproxy.net/run-id"
	for i, source := range sources {
		detailed := rig.GetConnectorVersionAsAdmin(t, source.Id, source.Version)
		definition := detailed.Definition
		definition.Id = apid.New(apid.PrefixConnectorVersion)
		definition.Version = 1
		definition.Name = common.ResourceName(fmt.Sprintf("%s-smoke-%d-%d", source.Name, suffix, i))
		namespace := smokeConnectorNamespace
		definition.Namespace = &namespace
		if source.Labels["demo.authproxy.net/seed-key"] == "demo-oauth-simple" {
			replacement := helpers.NewOAuth2Connector(definition.Id, string(definition.Name), provider, helpers.OAuth2ConnectorOptions{
				ClientID:     oauthClientID,
				ClientSecret: oauthClientSecret,
				Scopes:       []string{"read", "profile"},
			})
			definition.Auth = replacement.Auth
		}

		labels := make(map[string]string, len(source.Labels)+1)
		for key, value := range source.Labels {
			if !strings.HasPrefix(key, "apxy/") {
				labels[key] = value
			}
		}
		labels["smoke"] = "true"
		labels[runLabel] = runID
		created := rig.CreateConnectorWithLabels(t, definition, labels)
		rig.ForceConnectorVersionState(t, created.Id, created.Version, string(api.ConnectorVersionStatePrimary))
		t.Cleanup(func() {
			rig.ForceConnectorVersionState(t, created.Id, created.Version, string(api.ConnectorVersionStateArchived))
		})
	}
	return runLabel + "=" + runID
}

func TestRemoteOAuth2ProxySmoke(t *testing.T) {
	if *smokeBaseURL == "" {
		t.Skip("set SMOKE_BASE_URL or pass -base-url")
	}
	if *smokeGlobalKey == "" {
		t.Skip("set SMOKE_GLOBAL_KEY or pass -global-key")
	}

	rig := newRemoteSmokeRig(t)
	provider := helpers.NewOAuth2TestProviderAt(t, rig.ProviderURL)

	startedAt := time.Now().Add(-1 * time.Second)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "smoke-client-" + suffix
	clientSecret := "smoke-secret-" + suffix
	userEmail := "smoke-user-" + suffix + "@example.com"

	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             rig.PublicURL + "/oauth2/callback",
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})
	require.Equal(t, clientKey, client.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: "p4ssw0rd-" + suffix,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "smoke-oauth2-"+suffix, provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})
	created := rig.CreateConnector(t, connector)
	require.Equal(t, uint64(1), created.Version)
	rig.ForceConnectorVersionState(t, created.Id, created.Version, string(api.ConnectorVersionStatePrimary))
	t.Cleanup(func() {
		rig.ForceConnectorVersionState(t, created.Id, created.Version, string(api.ConnectorVersionStateArchived))
	})

	connectionID, redirectURL := rig.InitiateOAuth2Connection(t, created.Id, rig.PublicURL+"/connections")
	require.NotEmpty(t, connectionID)

	authorizeURL := rig.FollowOAuth2Redirect(t, redirectURL)
	authorizeParams := parseQuery(t, authorizeURL)
	require.Equal(t, clientKey, authorizeParams.Get("client_id"))
	require.Equal(t, rig.PublicURL+"/oauth2/callback", authorizeParams.Get("redirect_uri"))
	require.NotEmpty(t, authorizeParams.Get("state"))

	callback := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    clientKey,
		UserID:      user.ID,
		RedirectURI: authorizeParams.Get("redirect_uri"),
		Scope:       authorizeParams.Get("scope"),
		State:       authorizeParams.Get("state"),
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, callback.RedirectURL)

	finalLocation := rig.DeliverOAuth2Callback(t, callback.RedirectURL)
	assert.Truef(t, strings.HasPrefix(finalLocation, rig.PublicURL+"/connections"),
		"expected callback to land on marketplace connections page, got %q", finalLocation)

	proxyResp := rig.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), http.MethodGet)
	require.Equal(t, http.StatusOK, proxyResp.StatusCode)

	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
		Since:    startedAt,
	})
	require.NotEmpty(t, tokenReqs, "provider should record the token exchange")
	assert.Equal(t, "authorization_code", lastFormValue(tokenReqs[len(tokenReqs)-1].Form, "grant_type"))

	resourceReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointResource,
		Since:    startedAt,
	})
	require.NotEmpty(t, resourceReqs, "provider should record the proxied resource call")
	authHeader := resourceReqs[len(resourceReqs)-1].Headers["Authorization"]
	if authHeader == "" {
		authHeader = resourceReqs[len(resourceReqs)-1].Headers["authorization"]
	}
	require.Truef(t, strings.HasPrefix(strings.ToLower(authHeader), "bearer "),
		"proxied resource call should use bearer auth, got %q", authHeader)
}

func TestRemoteSeededOAuthConnectorSmoke(t *testing.T) {
	if *smokeBaseURL == "" {
		t.Skip("set SMOKE_BASE_URL or pass -base-url")
	}
	if *smokeGlobalKey == "" {
		t.Skip("set SMOKE_GLOBAL_KEY or pass -global-key")
	}

	rig := newRemoteSmokeRig(t)
	provider := helpers.NewOAuth2TestProviderAt(t, rig.ProviderURL)
	connector := rig.FindConnectorBySeedKey(t, "demo-oauth-simple")

	startedAt := time.Now().Add(-1 * time.Second)
	connectionID, redirectURL := rig.InitiateOAuth2Connection(t, connector.Id, rig.PublicURL+"/connections")
	authorizeURL := rig.FollowOAuth2Redirect(t, redirectURL)
	authorizeParams := parseQuery(t, authorizeURL)
	clientID := authorizeParams.Get("client_id")
	require.NotEmpty(t, clientID)
	require.Equal(t, rig.PublicURL+"/oauth2/callback", authorizeParams.Get("redirect_uri"))
	require.NotEmpty(t, authorizeParams.Get("state"))

	callback := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    clientID,
		Username:    "demo-oauth-user@example.test",
		RedirectURI: authorizeParams.Get("redirect_uri"),
		Scope:       authorizeParams.Get("scope"),
		State:       authorizeParams.Get("state"),
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, callback.RedirectURL)

	finalLocation := rig.DeliverOAuth2Callback(t, callback.RedirectURL)
	assert.Truef(t, strings.HasPrefix(finalLocation, rig.PublicURL+"/connections"),
		"expected callback to land on marketplace connections page, got %q", finalLocation)
	rig.WaitForSetupComplete(t, connectionID, 30*time.Second)

	proxyResp := rig.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), http.MethodGet)
	require.Equal(t, http.StatusOK, proxyResp.StatusCode)

	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientID,
		Since:    startedAt,
	})
	require.NotEmpty(t, tokenReqs, "provider should record the seeded client token exchange")
}

func TestRemoteSeededConnectorsSmoke(t *testing.T) {
	if *smokeBaseURL == "" {
		t.Skip("set SMOKE_BASE_URL or pass -base-url")
	}
	if *smokeGlobalKey == "" {
		t.Skip("set SMOKE_GLOBAL_KEY or pass -global-key")
	}

	rig := newRemoteSmokeRig(t)
	provider := helpers.NewOAuth2TestProviderAt(t, rig.ProviderURL)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	oauthClientID := "seeded-smoke-client-" + suffix
	oauthClientSecret := "seeded-smoke-secret-" + suffix
	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     oauthClientID,
		Secret:                  oauthClientSecret,
		RedirectURI:             rig.PublicURL + "/oauth2/callback",
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read profile resources",
	})
	require.Equal(t, oauthClientID, client.Key)

	runSelector := cloneSeededConnectorsIntoSmokeNamespace(t, rig, provider, oauthClientID, oauthClientSecret)
	items := rig.ListConnectors(t, runSelector)
	bySeedKey := map[string]api.ConnectorJson{}
	for _, item := range items {
		if key := item.Labels["demo.authproxy.net/seed-key"]; key != "" {
			bySeedKey[key] = item
		}
	}

	t.Run("catalog contains expected seeded connectors", func(t *testing.T) {
		expected := map[string]string{
			"demo-readme-noauth":   "Demo README Resource",
			"demo-api-key-bearer":  "Demo API Key: Bearer Token",
			"demo-oauth-simple":    "Demo OAuth: Basic Authorization",
			"demo-oauth-tenant":    "Demo OAuth: Tenant Selection",
			"demo-oauth-configure": "Demo OAuth: Resource Configuration",
		}
		for seedKey, displayName := range expected {
			connector, ok := bySeedKey[seedKey]
			require.Truef(t, ok, "missing seeded connector %q (%s); present seed keys: %v", seedKey, displayName, sortedKeys(bySeedKey))
			require.Equalf(t, displayName, connector.DisplayName, "seeded connector %q display name changed", seedKey)
			require.Equalf(t, api.ConnectorVersionStatePrimary, connector.State, "seeded connector %q should be primary", seedKey)
		}
	})

	t.Run("seeded basic OAuth connector completes and proxies", func(t *testing.T) {
		startedAt := time.Now().Add(-1 * time.Second)
		connector, ok := bySeedKey["demo-oauth-simple"]
		require.True(t, ok, "isolated seeded OAuth connector was not created")
		connectionID, redirectURL := rig.InitiateOAuth2Connection(t, connector.Id, rig.PublicURL+"/connections")

		authorizeURL := rig.FollowOAuth2Redirect(t, redirectURL)
		authorizeParams := parseQuery(t, authorizeURL)
		require.Equal(t, oauthClientID, authorizeParams.Get("client_id"))
		require.Equal(t, rig.PublicURL+"/oauth2/callback", authorizeParams.Get("redirect_uri"))
		require.NotEmpty(t, authorizeParams.Get("state"))

		callback := provider.Authorize(helpers.AuthorizeRequest{
			ClientID:    oauthClientID,
			Username:    "demo-oauth-user@example.test",
			RedirectURI: authorizeParams.Get("redirect_uri"),
			Scope:       authorizeParams.Get("scope"),
			State:       authorizeParams.Get("state"),
			Decision:    helpers.AuthorizeApprove,
		})
		require.NotEmpty(t, callback.RedirectURL)

		finalLocation := rig.DeliverOAuth2Callback(t, callback.RedirectURL)
		assert.Truef(t, strings.HasPrefix(finalLocation, rig.PublicURL+"/connections"),
			"expected callback to land on marketplace connections page, got %q", finalLocation)
		rig.WaitForSetupComplete(t, connectionID, 30*time.Second)

		proxyResp := rig.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), http.MethodGet)
		require.Equal(t, http.StatusOK, proxyResp.StatusCode)

		tokenReqs := provider.Requests(helpers.RequestsFilter{
			Endpoint: helpers.EndpointToken,
			ClientID: oauthClientID,
			Since:    startedAt,
		})
		require.NotEmpty(t, tokenReqs, "provider should record seeded OAuth token exchange")
	})

	t.Run("seeded API key connector completes and proxies", func(t *testing.T) {
		connector, ok := bySeedKey["demo-api-key-bearer"]
		require.True(t, ok, "isolated seeded API-key connector was not created")
		connectionID, stepID := rig.InitiateAPIKeyConnection(t, connector.Id)
		respType := rig.SubmitAPIKeyCredentials(t, connectionID, stepID, "demo-api-key")
		switch respType {
		case api.ConnectionSetupResponseTypeComplete:
		case api.ConnectionSetupResponseTypeVerifying:
			rig.WaitForSetupComplete(t, connectionID, 2*time.Minute)
		default:
			require.Failf(t, "unexpected API-key submit response", "got setup response type %q", respType)
		}

		proxyResp := rig.DoProxyRequest(t, connectionID, rig.ProviderURL+"/test/api-key-resource/demo-api-key", http.MethodGet)
		require.Equal(t, http.StatusOK, proxyResp.StatusCode)
		body, ok := proxyResp.BodyJson.(map[string]any)
		require.Truef(t, ok, "expected JSON body from API-key resource, got %#v", proxyResp.BodyJson)
		require.Equal(t, "api-key", body["auth"])
		require.Equal(t, "/test/api-key-resource/demo-api-key", body["path"])
	})
}

func parseQuery(t *testing.T, rawURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed.Query()
}

func sortedKeys(connectors map[string]api.ConnectorJson) []string {
	keys := make([]string, 0, len(connectors))
	for key := range connectors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func lastFormValue(form map[string][]string, key string) string {
	values := form[key]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
