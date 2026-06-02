//go:build smoke

package smoke

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	smokeBaseURL  = flag.String("base-url", os.Getenv("SMOKE_BASE_URL"), "base AuthProxy demo URL, e.g. https://demo.authproxy.net")
	smokeAdminKey = flag.String("admin-key", os.Getenv("SMOKE_ADMIN_KEY"), "admin actor private key PEM, or a path to it")
)

func TestRemoteOAuth2ProxySmoke(t *testing.T) {
	if *smokeBaseURL == "" {
		t.Skip("set SMOKE_BASE_URL or pass -base-url")
	}
	if *smokeAdminKey == "" {
		t.Skip("set SMOKE_ADMIN_KEY or pass -admin-key")
	}

	rig := helpers.NewRemoteAuthProxy(t, helpers.RemoteAuthProxyOptions{
		BaseURL:         *smokeBaseURL,
		AdminPrivateKey: *smokeAdminKey,
	})
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

func parseQuery(t *testing.T, rawURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed.Query()
}

func lastFormValue(form map[string][]string, key string) string {
	values := form[key]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
