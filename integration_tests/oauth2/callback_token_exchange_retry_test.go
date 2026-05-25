//go:build integration

package oauth2

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenExchangeRetryRig is the per-test fixture for the transient
// retry/5xx cases. Mirrors tokenExchangeFailureRig (PR B) but each test
// expects the proxy to make multiple /token calls before settling on a
// final outcome — success after a retried failure, or auth_failed after
// the retry budget is exhausted.
type tokenExchangeRetryRig struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	logCapture   *helpers.LogCapture
	clientKey    string
	userID       string
	connectorID  apid.ID
	scopes       []string
	returnToURL  string
	errorPageURL string
}

func newTokenExchangeRetryRig(t *testing.T, name string) *tokenExchangeRetryRig {
	t.Helper()
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
	})
	t.Cleanup(env.Cleanup)

	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &tokenExchangeRetryRig{
		provider:     provider,
		env:          env,
		logCapture:   logCapture,
		clientKey:    clientKey,
		userID:       user.ID,
		connectorID:  connectorID,
		scopes:       []string{"read"},
		returnToURL:  "https://example.com/return",
		errorPageURL: env.Cfg.GetRoot().ErrorPages.InternalError,
	}
}

func (r *tokenExchangeRetryRig) initiateAndMintCode(t *testing.T) (connectionID, stateID, code string) {
	t.Helper()
	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID = parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "InitiateOAuth2Connection should embed state_id: %s", redirectURL)

	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(r.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)
	pc, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code = pc.Query().Get("code")
	require.NotEmptyf(t, code, "provider should issue a code on approve; got %s", authResp.RedirectURL)
	return connID, stateID, code
}

func (r *tokenExchangeRetryRig) tokenCallCount() int {
	return len(r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: r.clientKey,
	}))
}

// TestTokenExchange_TransientRetrySucceeds — provider returns 503 on the
// first two /token calls, succeeds on the third. The proxy's retry
// budget (3 attempts) is exactly enough to ride through the transient
// outage. Asserts the connection ends up in the success state, with a
// real token row, and no rejection event is emitted — the eventual
// success means there was no failure to alert on, even though the
// provider was unhealthy for two attempts.
func TestTokenExchange_TransientRetrySucceeds(t *testing.T) {
	rig := newTokenExchangeRetryRig(t, "te-retry-success")
	connID, stateID, code := rig.initiateAndMintCode(t)

	// FailCount=2 means the next two /token calls return this scripted
	// 503; the third call falls through to default behavior (a real
	// access_token grant). The proxy's max-attempts=3 budget means we
	// arrive at the success on the final allowed try.
	rig.provider.Script(rig.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		Status:    503,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 2,
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))

	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"successful retry should land on return_to_url; got %q", loc)

	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, conn.State,
		"retried success must transition the connection to ready")
	assert.Nil(t, conn.SetupStep, "successful retry must not record an auth_failed setup_step")
	assert.Nil(t, conn.SetupError, "successful retry must not record a setup_error")

	require.NotNil(t, rig.env.GetOAuth2Token(t, connID),
		"retried success must persist the access token from the third attempt")

	// Most important assertion: 3 /token calls observed, proving the
	// retry loop actually retried twice. If the proxy gave up after
	// the first 503, this would be 1.
	assert.Equal(t, 3, rig.tokenCallCount(),
		"expected 3 /token calls (2 retried 503s + 1 success)")

	// Successful retry must not emit a token-exchange-failed event —
	// alert dashboards key on this message string and a noisy emit on
	// every transient blip would render them useless. Per-retry Warn
	// log lines are fine; the structured failure event is the one to
	// keep clean.
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage),
		"successful retry must not emit a token-exchange-failed event")
}

// TestTokenExchange_TransientRetryExhausted — provider returns 503 on
// every /token call. The proxy retries until the budget is exhausted,
// then settles into auth_failed. The single failure event records the
// final attempt count so dashboards can see "we tried 3 times and gave
// up" without parsing log lines.
func TestTokenExchange_TransientRetryExhausted(t *testing.T) {
	rig := newTokenExchangeRetryRig(t, "te-retry-exhausted")
	connID, stateID, code := rig.initiateAndMintCode(t)

	// FailCount=10 is well past the proxy's retry budget — we expect
	// the proxy to make exactly tokenExchangeMaxAttempts (=3) calls
	// before giving up. The remaining 7 scripted 503s sit unconsumed
	// in the queue and don't affect the outcome.
	rig.provider.Script(rig.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		Status:    503,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))

	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"exhausted retry should still 302 to return_to_url with setup=pending so the UI re-renders; got %q", loc)
	parsed, err := url.Parse(loc)
	require.NoError(t, err)
	assert.Equal(t, "pending", parsed.Query().Get("setup"))
	assert.Equal(t, connID, parsed.Query().Get("connection_id"))

	events := rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
	require.Lenf(t, events, 1, "exhausted retry should emit exactly one failure event; got %d (%v)", len(events), events)
	assert.Equal(t, "provider_5xx", events[0]["category"])
	assert.Equal(t, stateID, events[0]["state_id"])
	assert.Equal(t, float64(503), events[0]["provider_status_code"])
	// attempts records the budget exhaustion so the alert can distinguish
	// this from a single non-retryable failure.
	assert.Equal(t, float64(3), events[0]["attempts"],
		"failure event should record attempts=tokenExchangeMaxAttempts on exhaustion")

	require.Nil(t, rig.env.GetOAuth2Token(t, connID))

	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateSetup, conn.State)
	require.NotNil(t, conn.SetupStep)
	assert.Truef(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"exhausted retry should land in auth_failed; got %q", conn.SetupStep.String())
	require.NotNil(t, conn.SetupError)
	assert.Containsf(t, *conn.SetupError, "received status code 503",
		"setup_error should mention the 503; got %q", *conn.SetupError)

	assert.Equal(t, 3, rig.tokenCallCount(),
		"proxy should make exactly tokenExchangeMaxAttempts (=3) /token calls before giving up")
}

// TestTokenExchange_5xxVariants_AllRetried — sanity check that the
// retry policy is keyed on the 5xx range, not on a specific status
// code. Issue #168 lists 500/502/503/504 as the transient failure
// status codes; the proxy classifies any of them as provider_5xx and
// retries. We exercise 504 specifically because gateway-timeout
// responses can otherwise look indistinguishable from an unreachable
// upstream — they are still 5xx and should still retry.
func TestTokenExchange_5xxVariants_AllRetried(t *testing.T) {
	rig := newTokenExchangeRetryRig(t, "te-retry-504")
	connID, stateID, code := rig.initiateAndMintCode(t)

	rig.provider.Script(rig.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		Status:    504,
		Body:      `{"error":"gateway_timeout"}`,
		FailCount: 10,
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"504 exhaustion should still redirect to return_to_url; got %q", loc)

	events := rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
	require.Lenf(t, events, 1, "504 exhaustion should emit exactly one failure event; got %d", len(events))
	assert.Equal(t, "provider_5xx", events[0]["category"],
		"504 should classify as provider_5xx, same as 503")
	assert.Equal(t, float64(504), events[0]["provider_status_code"])
	assert.Equal(t, float64(3), events[0]["attempts"],
		"504 must be retried like any other 5xx — attempts should hit max")

	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	assert.Equal(t, 3, rig.tokenCallCount())
}

