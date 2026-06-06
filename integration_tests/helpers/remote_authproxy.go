package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

type RemoteAuthProxyOptions struct {
	BaseURL string

	AdminURL    string
	PublicURL   string
	ProviderURL string

	AdminActorExternalID string
	UserActorExternalID  string
	Namespace            string

	AdminPrivateKey string
}

type RemoteAuthProxy struct {
	AdminURL    string
	PublicURL   string
	ProviderURL string

	AdminActorExternalID string
	UserActorExternalID  string
	Namespace            string

	privateKey string
	client     *http.Client
}

type remoteResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func NewRemoteAuthProxy(t *testing.T, opts RemoteAuthProxyOptions) *RemoteAuthProxy {
	t.Helper()

	require.NotEmpty(t, opts.BaseURL, "base URL is required")
	require.NotEmpty(t, opts.AdminPrivateKey, "admin private key is required")

	adminURL := opts.AdminURL
	if adminURL == "" {
		adminURL = mustDeriveSubdomainURL(t, opts.BaseURL, "admin")
	}
	publicURL := opts.PublicURL
	if publicURL == "" {
		publicURL = mustDeriveSubdomainURL(t, opts.BaseURL, "marketplace")
	}
	providerURL := opts.ProviderURL
	if providerURL == "" {
		providerURL = mustDeriveSubdomainURL(t, opts.BaseURL, "oauth2")
	}

	adminActor := opts.AdminActorExternalID
	if adminActor == "" {
		adminActor = "demo-shell"
	}
	userActor := opts.UserActorExternalID
	if userActor == "" {
		userActor = "fresh-user"
	}
	namespace := opts.Namespace
	if namespace == "" {
		namespace = sconfig.RootNamespace
	}

	return &RemoteAuthProxy{
		AdminURL:             strings.TrimRight(adminURL, "/"),
		PublicURL:            strings.TrimRight(publicURL, "/"),
		ProviderURL:          strings.TrimRight(providerURL, "/"),
		AdminActorExternalID: adminActor,
		UserActorExternalID:  userActor,
		Namespace:            namespace,
		privateKey:           opts.AdminPrivateKey,
		client:               &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *RemoteAuthProxy) CreateActor(t *testing.T, externalID string, labels map[string]string) schemaapi.ActorJson {
	t.Helper()

	var actor schemaapi.ActorJson
	h.doSigned(t, h.AdminActorExternalID, http.MethodPost, h.AdminURL+"/api/v1/actors", schemaapi.CreateActorRequestJson{
		ExternalId: externalID,
		Namespace:  h.Namespace,
		Labels:     labels,
	}, true, http.StatusCreated, &actor)
	return actor
}

func (h *RemoteAuthProxy) CreateConnector(t *testing.T, connector sconfig.Connector) schemaapi.ConnectorVersionJson {
	t.Helper()

	var created schemaapi.ConnectorVersionJson
	h.doSigned(t, h.AdminActorExternalID, http.MethodPost, h.AdminURL+"/api/v1/connectors", schemaapi.CreateConnectorRequestJson{
		Namespace:  h.Namespace,
		Definition: connector,
		Labels: map[string]string{
			"smoke": "true",
		},
	}, true, http.StatusCreated, &created)
	return created
}

func (h *RemoteAuthProxy) ForceConnectorVersionState(t *testing.T, connectorID apid.ID, version uint64, state string) schemaapi.ConnectorVersionJson {
	t.Helper()

	var updated schemaapi.ConnectorVersionJson
	h.doSigned(
		t,
		h.AdminActorExternalID,
		http.MethodPut,
		fmt.Sprintf("%s/api/v1/connectors/%s/versions/%d/_force_state", h.AdminURL, connectorID, version),
		schemaapi.ForceConnectorVersionStateRequestJson{State: state},
		true,
		http.StatusOK,
		&updated,
	)
	return updated
}

func (h *RemoteAuthProxy) InitiateOAuth2Connection(t *testing.T, connectorID apid.ID, returnToURL string) (connectionID, redirectURL string) {
	t.Helper()

	var redirect schemaapi.ConnectionSetupRedirect
	h.doSigned(t, h.UserActorExternalID, http.MethodPost, h.PublicURL+"/api/v1/connections/_initiate", schemaapi.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		IntoNamespace: h.Namespace,
		ReturnToUrl:   returnToURL,
	}, true, http.StatusOK, &redirect)
	require.Equal(t, schemaapi.ConnectionSetupResponseTypeRedirect, redirect.Type)
	require.NotEmpty(t, redirect.RedirectUrl)
	return redirect.Id.String(), redirect.RedirectUrl
}

func (h *RemoteAuthProxy) FollowOAuth2Redirect(t *testing.T, redirectURL string) string {
	t.Helper()

	resp := h.doSigned(t, h.UserActorExternalID, http.MethodGet, redirectURL, nil, false, http.StatusFound, nil)
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc, "OAuth2 redirect response should include Location")
	return loc
}

func (h *RemoteAuthProxy) DeliverOAuth2Callback(t *testing.T, callbackURL string) string {
	t.Helper()

	resp := h.doSigned(t, h.UserActorExternalID, http.MethodGet, callbackURL, nil, false, http.StatusFound, nil)
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc, "OAuth2 callback response should include Location")
	return loc
}

func (h *RemoteAuthProxy) DoProxyRequest(t *testing.T, connectionID, targetURL, method string) schemaapi.ProxyResponseJson {
	t.Helper()

	var proxyResp schemaapi.ProxyResponseJson
	h.doSigned(t, h.UserActorExternalID, http.MethodPost, h.PublicURL+"/api/v1/connections/"+connectionID+"/_proxy", schemaapi.ProxyRequestJson{
		URL:    targetURL,
		Method: method,
	}, true, http.StatusOK, &proxyResp)
	return proxyResp
}

func (h *RemoteAuthProxy) doSigned(t *testing.T, actorExternalID, method, rawURL string, body any, followRedirects bool, wantStatus int, out any) remoteResponse {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err, "marshal request body")
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	signer, err := h.signer(actorExternalID)
	require.NoError(t, err, "build JWT signer for %s", actorExternalID)
	signer.SignAuthHeader(req)

	client := h.client
	if !followRedirects {
		client = &http.Client{
			Timeout: h.client.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	resp, err := client.Do(req)
	require.NoError(t, err, "%s %s", method, rawURL)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	require.Equalf(t, wantStatus, resp.StatusCode, "%s %s returned %d: %s", method, rawURL, resp.StatusCode, string(respBody))

	if out != nil {
		require.NoError(t, json.Unmarshal(respBody, out), "decode response from %s %s: %s", method, rawURL, string(respBody))
	}

	return remoteResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}
}

func (h *RemoteAuthProxy) signer(actorExternalID string) (jwt.Signer, error) {
	builder := jwt.NewJwtTokenBuilder().
		WithActorExternalId(actorExternalID).
		WithNamespace(h.Namespace).
		WithActorSigned().
		WithServiceIds(sconfig.AllServiceIds()).
		WithPermissions(aschema.AllPermissions()).
		WithExpiresIn(15 * time.Minute)

	if looksLikePath(h.privateKey) {
		builder = builder.WithPrivateKeyPath(h.privateKey)
	} else {
		builder = builder.WithPrivateKeyString(h.privateKey)
	}

	return builder.Signer()
}

func mustDeriveSubdomainURL(t *testing.T, baseURL, subdomain string) string {
	t.Helper()
	derived, err := deriveSubdomainURL(baseURL, subdomain)
	require.NoError(t, err)
	return derived
}

func mustDerivePathURL(t *testing.T, baseURL, path string) string {
	t.Helper()
	derived, err := derivePathURL(baseURL, path)
	require.NoError(t, err)
	return derived
}

func deriveSubdomainURL(baseURL, subdomain string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host: %q", baseURL)
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("base URL must include host: %q", baseURL)
	}
	if !strings.HasPrefix(host, subdomain+".") {
		host = subdomain + "." + host
	}

	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func derivePathURL(baseURL, path string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host: %q", baseURL)
	}

	u.Path = "/" + strings.Trim(path, "/")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func looksLikePath(value string) bool {
	if strings.Contains(value, "-----BEGIN ") {
		return false
	}
	if strings.Contains(value, "\n") {
		return false
	}
	_, err := os.Stat(value)
	return err == nil
}
