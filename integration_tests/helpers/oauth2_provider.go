package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// defaultOAuth2TestProviderURL is the host-side address of the go-oauth2-server
// container brought up by integration_tests/docker-compose.yml.
const defaultOAuth2TestProviderURL = "http://127.0.0.1:8086"

// OAuth2TestProvider is a thin client over the rmorlok/go-oauth2-server
// `--test-mode` control plane (`/test/*`). It lets a test register clients and
// users, drive the authorize step programmatically, script responses on
// recordable endpoints, and inspect/snapshot what the proxy sent.
//
// The full API is documented at:
//
//	https://github.com/rmorlok/go-oauth2-server/blob/main/docs/test_mode_api.md
type OAuth2TestProvider struct {
	t       *testing.T
	BaseURL string
	client  *http.Client
}

// NewOAuth2TestProvider connects to the test-mode OAuth2 server and waits for
// /test/health to come up. The URL defaults to http://127.0.0.1:8086 (the
// docker-compose port mapping) and may be overridden via the
// OAUTH2_TEST_PROVIDER_URL environment variable.
//
// It registers a t.Cleanup that clears any queued scripts at the end of the
// test so leftover state does not bleed into the next test.
func NewOAuth2TestProvider(t *testing.T) *OAuth2TestProvider {
	t.Helper()

	base := os.Getenv("OAUTH2_TEST_PROVIDER_URL")
	if base == "" {
		base = defaultOAuth2TestProviderURL
	}

	p := &OAuth2TestProvider{
		t:       t,
		BaseURL: base,
		client:  &http.Client{Timeout: 10 * time.Second},
	}

	p.waitHealthy(20 * time.Second)

	t.Cleanup(func() {
		_ = p.clearAllScripts()
	})

	return p
}

func (p *OAuth2TestProvider) waitHealthy(d time.Duration) {
	p.t.Helper()
	deadline := time.Now().Add(d)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := p.client.Get(p.BaseURL + "/test/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.FailNowf(p.t, "OAuth2 test provider not healthy",
		"GET %s/test/health: %v (is `docker compose up -d` running?)", p.BaseURL, lastErr)
}

// ----- Clients ---------------------------------------------------------------

// TokenEndpointAuthMethod values per RFC 7591 §2 as understood by the test
// provider.
type TokenEndpointAuthMethod string

const (
	TokenEndpointAuthBasic TokenEndpointAuthMethod = "client_secret_basic"
	TokenEndpointAuthPost  TokenEndpointAuthMethod = "client_secret_post"
	TokenEndpointAuthNone  TokenEndpointAuthMethod = "none"
)

type CreateClientRequest struct {
	Key                     string                  `json:"key"`
	Secret                  string                  `json:"secret,omitempty"`
	RedirectURI             string                  `json:"redirect_uri,omitempty"`
	TokenEndpointAuthMethod TokenEndpointAuthMethod `json:"token_endpoint_auth_method,omitempty"`
	RequirePKCE             bool                    `json:"require_pkce,omitempty"`
	Scope                   string                  `json:"scope,omitempty"`
}

type Client struct {
	ID                      string                  `json:"id"`
	Key                     string                  `json:"key"`
	RedirectURI             string                  `json:"redirect_uri"`
	TokenEndpointAuthMethod TokenEndpointAuthMethod `json:"token_endpoint_auth_method"`
	RequirePKCE             bool                    `json:"require_pkce"`
}

func (p *OAuth2TestProvider) CreateClient(req CreateClientRequest) Client {
	p.t.Helper()
	var out Client
	p.do("POST", "/test/clients", req, http.StatusCreated, &out)
	return out
}

// ----- Users -----------------------------------------------------------------

type CreateUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Role        string `json:"role,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Sub         string `json:"sub,omitempty"`
}

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Sub         string `json:"sub,omitempty"`
}

func (p *OAuth2TestProvider) CreateUser(req CreateUserRequest) User {
	p.t.Helper()
	var out User
	p.do("POST", "/test/users", req, http.StatusCreated, &out)
	return out
}

type UpdateIdentityRequest struct {
	Sub         *string `json:"sub,omitempty"`
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

func (p *OAuth2TestProvider) UpdateIdentity(userID string, req UpdateIdentityRequest) User {
	p.t.Helper()
	var out User
	p.do("POST", "/test/users/"+userID+"/identity", req, http.StatusOK, &out)
	return out
}

func (p *OAuth2TestProvider) SwapSubject(userID, newSub string) User {
	p.t.Helper()
	var out User
	p.do("POST", "/test/users/"+userID+"/swap-subject", map[string]string{"new_sub": newSub}, http.StatusOK, &out)
	return out
}

// ----- Authorize -------------------------------------------------------------

type AuthorizeDecision string

const (
	AuthorizeApprove AuthorizeDecision = "approve"
	AuthorizeDeny    AuthorizeDecision = "deny"
)

type AuthorizeRequest struct {
	ClientID            string            `json:"client_id"`
	UserID              string            `json:"user_id,omitempty"`
	Username            string            `json:"username,omitempty"`
	RedirectURI         string            `json:"redirect_uri,omitempty"`
	Scope               string            `json:"scope,omitempty"`
	State               string            `json:"state,omitempty"`
	Decision            AuthorizeDecision `json:"decision"`
	GrantedScope        string            `json:"granted_scope,omitempty"`
	CodeChallenge       string            `json:"code_challenge,omitempty"`
	CodeChallengeMethod string            `json:"code_challenge_method,omitempty"`
}

type AuthorizeResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// Authorize drives the authorize step programmatically and returns the redirect
// URL the proxy would have followed. Callers typically extract `code` and
// `state` from the URL's query.
func (p *OAuth2TestProvider) Authorize(req AuthorizeRequest) AuthorizeResponse {
	p.t.Helper()
	var out AuthorizeResponse
	p.do("POST", "/test/authorize", req, http.StatusOK, &out)
	return out
}

// ----- Scripted responses ----------------------------------------------------

// EndpointLabel identifies which recordable endpoint a script targets.
type EndpointLabel string

const (
	EndpointToken      EndpointLabel = "token"
	EndpointRefresh    EndpointLabel = "refresh"
	EndpointIntrospect EndpointLabel = "introspect"
	EndpointRevoke     EndpointLabel = "revoke"
	EndpointUserinfo   EndpointLabel = "userinfo"
	EndpointResource   EndpointLabel = "resource"
)

// BodyTemplate names a built-in response shape (see test_mode_api.md).
type BodyTemplate string

const (
	BodyAccessTokenSuccess        BodyTemplate = "access_token_success"
	BodyAccessTokenNoScope        BodyTemplate = "access_token_no_scope"
	BodyInvalidGrant              BodyTemplate = "invalid_grant"
	BodyTemporarilyUnavailable503 BodyTemplate = "temporarily_unavailable_503"
	BodyMalformedJSON             BodyTemplate = "malformed_json"
)

type ScriptAction struct {
	Status         int               `json:"status,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
	BodyTemplate   BodyTemplate      `json:"body_template,omitempty"`
	DelayMs        int               `json:"delay_ms,omitempty"`
	DropConnection bool              `json:"drop_connection,omitempty"`
	FailCount      int               `json:"fail_count,omitempty"`
	ScopeOverride  *string           `json:"scope_override,omitempty"`
	SkipPKCECheck  bool              `json:"skip_pkce_check,omitempty"`
}

type scriptRequest struct {
	ClientID string         `json:"client_id"`
	Endpoint EndpointLabel  `json:"endpoint"`
	Actions  []ScriptAction `json:"actions"`
}

// Script enqueues actions for (clientID, endpoint). Pass clientID="" for a
// wildcard queue that matches any caller.
func (p *OAuth2TestProvider) Script(clientID string, endpoint EndpointLabel, actions ...ScriptAction) {
	p.t.Helper()
	require.NotEmpty(p.t, actions, "Script requires at least one action")
	p.do("POST", "/test/scripts", scriptRequest{
		ClientID: clientID,
		Endpoint: endpoint,
		Actions:  actions,
	}, http.StatusNoContent, nil)
}

// ClearScripts removes queued actions matching the given filter. Empty
// arguments match anything.
func (p *OAuth2TestProvider) ClearScripts(clientID string, endpoint EndpointLabel) {
	p.t.Helper()
	q := url.Values{}
	if clientID != "" {
		q.Set("client_id", clientID)
	}
	if endpoint != "" {
		q.Set("endpoint", string(endpoint))
	}
	path := "/test/scripts"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	p.do("DELETE", path, nil, http.StatusNoContent, nil)
}

func (p *OAuth2TestProvider) clearAllScripts() error {
	req, err := http.NewRequest("DELETE", p.BaseURL+"/test/scripts", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

// ----- Revocation, rotation, resource policy --------------------------------

// RevokeToken provider-side: bypass client auth and ownership checks.
func (p *OAuth2TestProvider) RevokeToken(token string) {
	p.t.Helper()
	p.do("POST", "/test/revoke", map[string]string{"token": token}, http.StatusOK, nil)
}

// RevokeUser revokes all unrevoked tokens for the given user.
func (p *OAuth2TestProvider) RevokeUser(userID string) {
	p.t.Helper()
	p.do("POST", "/test/revoke", map[string]string{"user_id": userID}, http.StatusOK, nil)
}

// RevokeClient revokes all unrevoked tokens for the given client. Accepts
// either the public key or the database UUID.
func (p *OAuth2TestProvider) RevokeClient(clientID string) {
	p.t.Helper()
	p.do("POST", "/test/revoke", map[string]string{"client_id": clientID}, http.StatusOK, nil)
}

func (p *OAuth2TestProvider) SetRefreshRotation(on bool) {
	p.t.Helper()
	p.do("POST", "/test/refresh-tokens/rotate-policy", map[string]bool{"rotation": on}, http.StatusOK, nil)
}

func (p *OAuth2TestProvider) SetResourcePolicy(path, requiredScope string) {
	p.t.Helper()
	p.do("POST", "/test/resource-policy", map[string]string{
		"path":           path,
		"required_scope": requiredScope,
	}, http.StatusNoContent, nil)
}

// ----- Request inspection ----------------------------------------------------

type RecordedRequest struct {
	Timestamp time.Time           `json:"timestamp"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Endpoint  EndpointLabel       `json:"endpoint"`
	ClientID  string              `json:"client_id"`
	Headers   map[string]string   `json:"headers"`
	Query     map[string][]string `json:"query"`
	Form      map[string][]string `json:"form"`
}

type RequestsFilter struct {
	Endpoint EndpointLabel
	ClientID string
	Since    time.Time
}

// Requests returns a sanitized snapshot of recent requests matching the
// filter. The recorder is a bounded ring buffer (1000 entries).
func (p *OAuth2TestProvider) Requests(filter RequestsFilter) []RecordedRequest {
	p.t.Helper()
	q := url.Values{}
	if filter.Endpoint != "" {
		q.Set("endpoint", string(filter.Endpoint))
	}
	if filter.ClientID != "" {
		q.Set("client_id", filter.ClientID)
	}
	if !filter.Since.IsZero() {
		q.Set("since", filter.Since.UTC().Format(time.RFC3339Nano))
	}
	path := "/test/requests"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out []RecordedRequest
	p.do("GET", path, nil, http.StatusOK, &out)
	return out
}

// ----- URL helpers -----------------------------------------------------------

// AuthorizationEndpoint is the URL the proxy points its authorize step at.
// go-oauth2-server serves the human-facing authorize/consent flow under
// /web/authorize (GET shows the consent form, POST submits the decision);
// /v1/oauth/* hosts the machine endpoints (tokens, introspect, revoke).
func (p *OAuth2TestProvider) AuthorizationEndpoint() string {
	return p.BaseURL + "/web/authorize"
}

// TokenEndpoint is the URL for /v1/oauth/tokens.
func (p *OAuth2TestProvider) TokenEndpoint() string {
	return p.BaseURL + "/v1/oauth/tokens"
}

// RevocationEndpoint is the URL for the standard RFC 7009 endpoint.
func (p *OAuth2TestProvider) RevocationEndpoint() string {
	return p.BaseURL + "/v1/oauth/revoke"
}

// ResourceURL builds a URL for the sample resource at /test/resource/<path>.
func (p *OAuth2TestProvider) ResourceURL(path string) string {
	if path == "" {
		return p.BaseURL + "/test/resource/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return p.BaseURL + "/test/resource" + path
}

// ----- internal --------------------------------------------------------------

func (p *OAuth2TestProvider) do(method, path string, body any, wantStatus int, out any) {
	p.t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(p.t, err, "marshal request body")
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, p.BaseURL+path, reqBody)
	require.NoError(p.t, err, "build %s %s", method, path)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	require.NoError(p.t, err, "%s %s", method, path)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(p.t, wantStatus, resp.StatusCode,
		"%s %s: want %d, got %d, body=%s", method, path, wantStatus, resp.StatusCode, string(respBody))

	if out != nil && len(respBody) > 0 {
		require.NoErrorf(p.t, json.Unmarshal(respBody, out),
			"decode %s %s response: %s", method, path, string(respBody))
	}
}
