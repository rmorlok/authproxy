//go:build integration

// Package oauth2 holds integration tests that drive the rmorlok/go-oauth2-server
// test provider via the OAuth2TestProvider helper.
package oauth2

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHarness_HealthAndClientRegistration verifies the harness can reach the
// test provider, register a confidential client, and read it back via the
// returned record.
func TestHarness_HealthAndClientRegistration(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	clientKey := smokeClientKey(t, "confidential")
	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:         clientKey,
		Secret:      "shhh",
		RedirectURI: "https://app.example.com/cb",
	})

	require.NotEmpty(t, client.ID)
	assert.Equal(t, clientKey, client.Key)
	assert.Equal(t, "https://app.example.com/cb", client.RedirectURI)
	// Default auth method per RFC 7591 §2.
	assert.Equal(t, helpers.TokenEndpointAuthBasic, client.TokenEndpointAuthMethod)
}

// TestHarness_DriveAuthorizeApprove walks a client+user through the
// programmatic authorize endpoint and validates the redirect carries a code
// and the original state.
func TestHarness_DriveAuthorizeApprove(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	const redirectURI = "https://app.example.com/cb"
	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:         smokeClientKey(t, "authorize"),
		Secret:      "s3cret",
		RedirectURI: redirectURI,
	})
	userEmail := smokeUserEmail(t, "alice")

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: "p4ssw0rd",
		Email:    userEmail,
	})

	resp := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    client.Key,
		UserID:      user.ID,
		RedirectURI: redirectURI,
		Scope:       "read",
		State:       "csrf-abc",
		Decision:    helpers.AuthorizeApprove,
	})

	require.NotEmpty(t, resp.RedirectURL)
	require.True(t, strings.HasPrefix(resp.RedirectURL, redirectURI), "redirect should target client's redirect_uri")

	parsed, err := url.Parse(resp.RedirectURL)
	require.NoError(t, err)

	q := parsed.Query()
	assert.NotEmpty(t, q.Get("code"), "approve must include code")
	assert.Equal(t, "csrf-abc", q.Get("state"), "state must be echoed verbatim")
	assert.Empty(t, q.Get("error"), "approve must not include error")
}

// TestHarness_DriveAuthorizeDeny validates the deny path returns
// access_denied and echoes state without exchanging a code.
func TestHarness_DriveAuthorizeDeny(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:         smokeClientKey(t, "deny"),
		Secret:      "s",
		RedirectURI: "https://app.example.com/cb",
	})

	resp := provider.Authorize(helpers.AuthorizeRequest{
		ClientID: client.Key,
		State:    "deny-state",
		Decision: helpers.AuthorizeDeny,
	})

	parsed, err := url.Parse(resp.RedirectURL)
	require.NoError(t, err)
	q := parsed.Query()
	assert.Equal(t, "access_denied", q.Get("error"))
	assert.Equal(t, "deny-state", q.Get("state"))
	assert.Empty(t, q.Get("code"))
}

// TestHarness_ScriptAndInspect enqueues a scripted response, then verifies
// the request inspector records subsequent calls. We don't actually exchange
// a token here — that's covered by tests that drive the full proxy flow —
// but this exercises the script + inspector plumbing so future scenario
// tests can rely on them.
func TestHarness_ScriptAndInspect(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	client := provider.CreateClient(helpers.CreateClientRequest{
		Key:         smokeClientKey(t, "script"),
		Secret:      "s",
		RedirectURI: "https://app.example.com/cb",
	})

	// Enqueue a 503 on the next refresh call. We won't trigger it here; this
	// just validates that POST /test/scripts accepts a known body_template.
	provider.Script(client.Key, helpers.EndpointRefresh, helpers.ScriptAction{
		BodyTemplate: helpers.BodyTemporarilyUnavailable503,
	})

	// Clear it back out and confirm the call shape works for the cleanup
	// code path we register from t.Cleanup.
	provider.ClearScripts(client.Key, helpers.EndpointRefresh)

	// /test/requests should be reachable and return a slice (possibly empty).
	got := provider.Requests(helpers.RequestsFilter{ClientID: client.Key})
	assert.NotNil(t, got, "Requests should always return a slice (empty or otherwise)")
}

func smokeClientKey(t *testing.T, suffix string) string {
	t.Helper()
	testName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	return "smoke-" + suffix + "-" + strings.ToLower(testName) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func smokeUserEmail(t *testing.T, prefix string) string {
	t.Helper()
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
}
