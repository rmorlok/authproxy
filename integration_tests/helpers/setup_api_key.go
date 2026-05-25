package helpers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// ApiKeyConnectorOptions configures NewApiKeyConnector.
type ApiKeyConnectorOptions struct {
	// Placement selects how the api key is sent on outbound requests. One of
	// connectors.ApiKeyPlacementBearer/Header/Query/Basic.
	Placement connectors.ApiKeyPlacementType

	// HeaderName is required for header placement.
	HeaderName string
	// HeaderPrefix is the optional literal prepended to the key value for
	// header placement.
	HeaderPrefix string
	// ParamName is required for query placement.
	ParamName string
	// UsernameField is required for basic placement.
	UsernameField string

	// ProbeURL is the URL the connector's verify-time probe hits. Typically
	// the stub upstream's BaseURL plus a path. If empty, no probe is added —
	// the connection skips verify and lands ready immediately after submit.
	ProbeURL string
	// ProbeFailureThreshold overrides the probe's failure threshold. When
	// zero, the connector default applies (DefaultProbeFailureThreshold).
	// Tests that drive probe-driven health typically set this to 1 so a
	// single failed probe flips health_state.
	ProbeFailureThreshold *int
	// ProbeRecoveryThreshold overrides the probe's recovery threshold. When
	// zero, DefaultProbeRecoveryThreshold (1) applies.
	ProbeRecoveryThreshold *int
}

// NewApiKeyConnector builds an authproxy connector with the given api-key
// placement and optionally a single proxy_http probe pointing at probeURL.
// The probe path exercises the same authentication logic the real proxy
// uses, so verifying the probe passes is equivalent to verifying the
// credential was placed correctly on outbound requests.
func NewApiKeyConnector(connectorID apid.ID, displayName string, opts ApiKeyConnectorOptions) sconfig.Connector {
	placement := &connectors.ApiKeyPlacement{Type: opts.Placement}
	switch opts.Placement {
	case connectors.ApiKeyPlacementHeader:
		placement.HeaderName = opts.HeaderName
		placement.Prefix = opts.HeaderPrefix
	case connectors.ApiKeyPlacementQuery:
		placement.ParamName = opts.ParamName
	case connectors.ApiKeyPlacementBasic:
		placement.UsernameField = opts.UsernameField
	}

	c := sconfig.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": displayName},
		DisplayName: displayName,
		Auth: &connectors.Auth{
			InnerVal: &connectors.AuthApiKey{
				Type:      connectors.AuthTypeAPIKey,
				Placement: placement,
			},
		},
	}

	if opts.ProbeURL != "" {
		probe := connectors.Probe{
			Id: "verify-credential",
			ProxyHttp: &connectors.ProbeHttp{
				Method: "GET",
				URL:    opts.ProbeURL,
			},
			FailureThreshold:  opts.ProbeFailureThreshold,
			RecoveryThreshold: opts.ProbeRecoveryThreshold,
		}
		c.Probes = []connectors.Probe{probe}
	}

	// Normalize synthesizes the credentials setup-flow step on top of the
	// AuthApiKey placement so the connector definition is "ready to use" before
	// migration sees it. The migration path also normalizes via Validate, but
	// we call it here too so callers (and tests that inspect the returned
	// definition) see the same shape that lands in the database.
	c.Normalize()

	return c
}

// InitiateApiKeyConnection POSTs to /api/v1/connections/_initiate for an api-key
// connector. Unlike the OAuth2 path, the response is a form (the credentials
// step), not a redirect. Returns the new connection id and the form payload.
// Signed by default as actor "test-actor" in the root namespace; pass
// WithActor(...) to mirror a specific tenant.
func (env *IntegrationTestEnv) InitiateApiKeyConnection(t *testing.T, connectorID apid.ID, opts ...OAuth2Option) (connectionID string, form *coreIface.ConnectionSetupForm) {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "",
		"InitiateApiKeyConnection requires either in-process gin or a running HTTP server")

	cfg := env.resolveOAuth2Options(opts)

	body, err := jsonMarshal(coreIface.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		IntoNamespace: cfg.actorNamespace,
	})
	require.NoError(t, err)

	w := env.doSignedRequest(t, http.MethodPost, "/api/v1/connections/_initiate", body, cfg)
	require.Equalf(t, http.StatusOK, w.Code, "initiate failed: %s", w.Body.String())

	var generic struct {
		Type string `json:"type"`
		Id   string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &generic))
	require.Equal(t, string(coreIface.ConnectionSetupResponseTypeForm), generic.Type,
		"expected api-key connector to return form response (got %s): %s", generic.Type, w.Body.String())

	var resp coreIface.ConnectionSetupForm
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &resp))
	return resp.Id.String(), &resp
}

// SubmitApiKeyCredentials POSTs to /api/v1/connections/{id}/_submit with the
// supplied step id and field data. Returns the response body so callers can
// inspect what shape came back (form / verifying / complete).
func (env *IntegrationTestEnv) SubmitApiKeyCredentials(t *testing.T, connectionID, stepID string, data map[string]any, opts ...OAuth2Option) *httptest.ResponseRecorder {
	t.Helper()
	cfg := env.resolveOAuth2Options(opts)

	rawData, err := json.Marshal(data)
	require.NoError(t, err)
	body, err := jsonMarshal(coreIface.SubmitConnectionRequest{
		StepId: stepID,
		Data:   rawData,
	})
	require.NoError(t, err)

	return env.doSignedRequest(t, http.MethodPost,
		"/api/v1/connections/"+connectionID+"/_submit", body, cfg)
}

// ReauthConnection POSTs to /api/v1/connections/{id}/_reauth and returns the
// raw response. For api-key connectors the response is a form (credentials
// step) the user fills in to rotate the key.
func (env *IntegrationTestEnv) ReauthConnection(t *testing.T, connectionID string, opts ...OAuth2Option) *httptest.ResponseRecorder {
	t.Helper()
	cfg := env.resolveOAuth2Options(opts)

	// Body is optional for api-key; sending an empty struct exercises the
	// JSON unmarshal path on the route.
	body, err := jsonMarshal(struct{}{})
	require.NoError(t, err)

	return env.doSignedRequest(t, http.MethodPost,
		"/api/v1/connections/"+connectionID+"/_reauth", body, cfg)
}

// RunProbe synchronously invokes the named probe against the connection and
// records the outcome against health-state counters. Wraps the public iface
// method so tests don't need a background worker to exercise the
// probe-driven health path.
func (env *IntegrationTestEnv) RunProbe(t *testing.T, connectionID, probeID string) error {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	return env.Core.RunProbe(context.Background(), id, probeID)
}

// RunVerifyConnection synchronously runs the verify-phase probes for the
// connection. Used to advance a connection through the verify step without
// running a background worker. No-ops cleanly if the connection isn't in
// verify phase (matches the task handler's stale-task guard).
func (env *IntegrationTestEnv) RunVerifyConnection(t *testing.T, connectionID string) error {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	return env.Core.RunVerifyConnection(context.Background(), id)
}

// DecryptApiKeyCredential returns the plaintext api-key (and username, when
// basic) currently active on the connection. Used by tests to confirm
// rotation actually replaced the stored secret.
func (env *IntegrationTestEnv) DecryptApiKeyCredential(t *testing.T, connectionID string) database.ApiKeyCredentialPlaintext {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	cred, err := env.Db.GetActiveApiKeyCredential(context.Background(), id)
	require.NoError(t, err)
	plaintext, err := env.DM.GetEncryptService().DecryptString(context.Background(), cred.EncryptedCredentials)
	require.NoError(t, err)
	var out database.ApiKeyCredentialPlaintext
	require.NoError(t, json.Unmarshal([]byte(plaintext), &out))
	return out
}

// doSignedRequest signs an admin-namespace request as the configured actor
// and dispatches it through either the in-process gin engine or the real
// HTTP server, returning a recorder uniformly.
func (env *IntegrationTestEnv) doSignedRequest(t *testing.T, method, path string, body io.Reader, cfg oauth2Options) *httptest.ResponseRecorder {
	t.Helper()

	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		method,
		path,
		body,
		cfg.actorNamespace,
		cfg.actorExternalID,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	if env.ApiGin != nil {
		env.ApiGin.ServeHTTP(w, req)
		return w
	}

	abs, err := url.Parse(env.ServerURL + path)
	require.NoError(t, err)
	req.URL = abs
	req.Host = abs.Host
	req.RequestURI = ""
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	w.Code = resp.StatusCode
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if _, err := w.Body.ReadFrom(resp.Body); err != nil {
		require.NoError(t, err)
	}
	return w
}

// ApiKeySubmitFormStepId returns the step id used by api-key connectors that
// don't declare their own credentials step. The runtime synthesizes a step
// with this id; tests must echo it back on submit. Exported to keep tests
// from hardcoding the magic string.
func ApiKeySubmitFormStepId() string {
	return connectors.SynthesizedApiKeyCredentialsStepId
}

// ContainsBytes is a small helper for the no-replay assertion — checks
// whether the supplied raw bytes appear in any header value of any
// recorded stub-upstream request. Returns the first offending request
// for use in failure messages.
func ContainsBytes(reqs []ApiKeyStubRequest, needle string) (bool, ApiKeyStubRequest) {
	for _, r := range reqs {
		if strings.Contains(r.RawQuery, needle) {
			return true, r
		}
		for _, vs := range r.Headers {
			for _, v := range vs {
				if strings.Contains(v, needle) {
					return true, r
				}
			}
		}
	}
	return false, ApiKeyStubRequest{}
}
