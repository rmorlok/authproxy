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
	AdminActorNamespace  string
	UserActorExternalID  string
	UserActorNamespace   string
	ConnectorNamespace   string
	ConnectionNamespace  string

	AdminPrivateKey string
}

type RemoteAuthProxy struct {
	AdminURL    string
	PublicURL   string
	ProviderURL string

	AdminActorExternalID string
	AdminActorNamespace  string
	UserActorExternalID  string
	UserActorNamespace   string
	ConnectorNamespace   string
	ConnectionNamespace  string

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
	adminActorNamespace := opts.AdminActorNamespace
	if adminActorNamespace == "" {
		adminActorNamespace = sconfig.RootNamespace
	}
	userActor := opts.UserActorExternalID
	if userActor == "" {
		userActor = "fresh-user"
	}
	userActorNamespace := opts.UserActorNamespace
	if userActorNamespace == "" {
		userActorNamespace = sconfig.RootNamespace
	}
	connectorNamespace := opts.ConnectorNamespace
	if connectorNamespace == "" {
		connectorNamespace = sconfig.RootNamespace
	}
	connectionNamespace := opts.ConnectionNamespace
	if connectionNamespace == "" {
		connectionNamespace = userActorNamespace
	}

	return &RemoteAuthProxy{
		AdminURL:             strings.TrimRight(adminURL, "/"),
		PublicURL:            strings.TrimRight(publicURL, "/"),
		ProviderURL:          strings.TrimRight(providerURL, "/"),
		AdminActorExternalID: adminActor,
		AdminActorNamespace:  adminActorNamespace,
		UserActorExternalID:  userActor,
		UserActorNamespace:   userActorNamespace,
		ConnectorNamespace:   connectorNamespace,
		ConnectionNamespace:  connectionNamespace,
		privateKey:           opts.AdminPrivateKey,
		client:               &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *RemoteAuthProxy) EnsureNamespace(t *testing.T, namespace string) {
	t.Helper()

	endpoint := h.AdminURL + "/api/v1/namespaces/" + url.PathEscape(namespace)
	resp := h.doSignedAllowing(t, h.AdminActorExternalID, h.AdminActorNamespace, http.MethodGet, endpoint, nil, true, []int{http.StatusOK, http.StatusNotFound}, nil)
	if resp.StatusCode == http.StatusOK {
		return
	}

	h.doSignedAllowing(t, h.AdminActorExternalID, h.AdminActorNamespace, http.MethodPost, h.AdminURL+"/api/v1/namespaces", schemaapi.CreateNamespaceRequestJson{
		Path: namespace,
	}, true, []int{http.StatusOK, http.StatusConflict}, nil)
}

func (h *RemoteAuthProxy) GetActorByExternalID(t *testing.T, namespace, externalID string) schemaapi.ActorJson {
	t.Helper()

	endpoint := h.AdminURL + "/api/v1/actors/external-id/" + url.PathEscape(externalID) + "?namespace=" + url.QueryEscape(namespace)
	var actor schemaapi.ActorJson
	h.doSigned(t, h.AdminActorExternalID, h.AdminActorNamespace, http.MethodGet, endpoint, nil, true, http.StatusOK, &actor)
	return actor
}

func (h *RemoteAuthProxy) WaitForUserReady(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last remoteResponse
	for time.Now().Before(deadline) {
		last = h.doSignedAllowing(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, h.PublicURL+"/api/v1/connectors?limit=1", nil, true, []int{http.StatusOK, http.StatusUnauthorized}, nil)
		if last.StatusCode == http.StatusOK {
			return
		}
		time.Sleep(1 * time.Second)
	}
	require.FailNowf(t, "smoke user did not become ready", "actor %q in namespace %q was not ready within %s; last response: %s", h.UserActorExternalID, h.UserActorNamespace, timeout, string(last.Body))
}

func (h *RemoteAuthProxy) CreateConnector(t *testing.T, connector sconfig.Connector) schemaapi.ConnectorVersionJson {
	t.Helper()
	return h.CreateConnectorWithLabels(t, connector, map[string]string{"smoke": "true"})
}

func (h *RemoteAuthProxy) CreateConnectorWithLabels(t *testing.T, connector sconfig.Connector, labels map[string]string) schemaapi.ConnectorVersionJson {
	t.Helper()

	var created schemaapi.ConnectorVersionJson
	h.doSigned(t, h.AdminActorExternalID, h.AdminActorNamespace, http.MethodPost, h.AdminURL+"/api/v1/connectors", schemaapi.CreateConnectorRequestJson{
		Namespace:  h.ConnectorNamespace,
		Definition: connector,
		Labels:     labels,
	}, true, http.StatusCreated, &created)
	require.Equal(t, h.ConnectorNamespace, created.Namespace)
	return created
}

func (h *RemoteAuthProxy) ListConnectorsAsAdmin(t *testing.T, namespace, labelSelector string) []schemaapi.ConnectorJson {
	t.Helper()

	query := url.Values{"limit": []string{"100"}, "namespace": []string{namespace}}
	if labelSelector != "" {
		query.Set("labelSelector", labelSelector)
	}
	endpoint := h.AdminURL + "/api/v1/connectors?" + query.Encode()

	var list schemaapi.ListConnectorsResponseJson
	h.doSigned(t, h.AdminActorExternalID, h.AdminActorNamespace, http.MethodGet, endpoint, nil, true, http.StatusOK, &list)
	return list.Items
}

func (h *RemoteAuthProxy) GetConnectorVersionAsAdmin(t *testing.T, connectorID apid.ID, version uint64) schemaapi.ConnectorVersionJson {
	t.Helper()

	var connector schemaapi.ConnectorVersionJson
	h.doSigned(
		t,
		h.AdminActorExternalID,
		h.AdminActorNamespace,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/connectors/%s/versions/%d", h.AdminURL, connectorID, version),
		nil,
		true,
		http.StatusOK,
		&connector,
	)
	return connector
}

func (h *RemoteAuthProxy) ListConnectors(t *testing.T, labelSelector string) []schemaapi.ConnectorJson {
	t.Helper()

	endpoint := h.PublicURL + "/api/v1/connectors?limit=100"
	if labelSelector != "" {
		endpoint += "&labelSelector=" + url.QueryEscape(labelSelector)
	}

	var list schemaapi.ListConnectorsResponseJson
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, endpoint, nil, true, http.StatusOK, &list)
	return list.Items
}

func (h *RemoteAuthProxy) FindConnectorBySeedKey(t *testing.T, seedKey string) schemaapi.ConnectorJson {
	t.Helper()

	connectors := h.ListConnectors(t, "demo.authproxy.net/seed-key="+seedKey)
	require.Lenf(t, connectors, 1, "expected exactly one seeded connector with key %q; got %d", seedKey, len(connectors))
	return connectors[0]
}

func (h *RemoteAuthProxy) ForceConnectorVersionState(t *testing.T, connectorID apid.ID, version uint64, state string) schemaapi.ConnectorVersionJson {
	t.Helper()

	var updated schemaapi.ConnectorVersionJson
	h.doSigned(
		t,
		h.AdminActorExternalID,
		h.AdminActorNamespace,
		http.MethodPut,
		fmt.Sprintf("%s/api/v1/connectors/%s/versions/%d/_forceState", h.AdminURL, connectorID, version),
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
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodPost, h.PublicURL+"/api/v1/connections/_initiate", schemaapi.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		IntoNamespace: h.ConnectionNamespace,
		ReturnToUrl:   returnToURL,
	}, true, http.StatusOK, &redirect)
	require.Equal(t, schemaapi.ConnectionSetupResponseTypeRedirect, redirect.Type)
	require.NotEmpty(t, redirect.RedirectUrl)
	require.Equal(t, h.ConnectionNamespace, h.GetConnection(t, redirect.Id.String()).Namespace)
	return redirect.Id.String(), redirect.RedirectUrl
}

func (h *RemoteAuthProxy) InitiateAPIKeyConnection(t *testing.T, connectorID apid.ID) (connectionID, stepID string) {
	t.Helper()

	var form schemaapi.ConnectionSetupForm
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodPost, h.PublicURL+"/api/v1/connections/_initiate", schemaapi.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		IntoNamespace: h.ConnectionNamespace,
	}, true, http.StatusOK, &form)
	require.Equal(t, schemaapi.ConnectionSetupResponseTypeForm, form.Type)
	require.NotEmpty(t, form.StepId)
	require.Equal(t, h.ConnectionNamespace, h.GetConnection(t, form.Id.String()).Namespace)
	return form.Id.String(), form.StepId
}

func (h *RemoteAuthProxy) GetConnection(t *testing.T, connectionID string) schemaapi.ConnectionJson {
	t.Helper()

	var connection schemaapi.ConnectionJson
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, h.PublicURL+"/api/v1/connections/"+connectionID, nil, true, http.StatusOK, &connection)
	return connection
}

func (h *RemoteAuthProxy) SubmitAPIKeyCredentials(t *testing.T, connectionID, stepID, apiKey string) schemaapi.ConnectionSetupResponseType {
	t.Helper()

	rawData, err := json.Marshal(map[string]string{"api_key": apiKey})
	require.NoError(t, err)

	var generic struct {
		Type schemaapi.ConnectionSetupResponseType `json:"type"`
	}
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodPost, h.PublicURL+"/api/v1/connections/"+connectionID+"/_submit", schemaapi.SubmitConnectionRequest{
		StepId: stepID,
		Data:   rawData,
	}, true, http.StatusOK, &generic)
	require.NotEmpty(t, generic.Type)
	return generic.Type
}

func (h *RemoteAuthProxy) WaitForSetupComplete(t *testing.T, connectionID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastType schemaapi.ConnectionSetupResponseType
	var lastError string
	for time.Now().Before(deadline) {
		var generic struct {
			Type  schemaapi.ConnectionSetupResponseType `json:"type"`
			Error string                                `json:"error,omitempty"`
		}
		h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, h.PublicURL+"/api/v1/connections/"+connectionID+"/_setupStep", nil, true, http.StatusOK, &generic)
		lastType = generic.Type
		lastError = generic.Error

		switch generic.Type {
		case schemaapi.ConnectionSetupResponseTypeComplete:
			return
		case schemaapi.ConnectionSetupResponseTypeError:
			require.FailNowf(t, "connection setup failed", "connection %s setup error: %s", connectionID, generic.Error)
		}
		time.Sleep(1 * time.Second)
	}
	require.FailNowf(t, "connection setup did not complete",
		"connection %s did not complete within %s; last type=%q error=%q", connectionID, timeout, lastType, lastError)
}

func (h *RemoteAuthProxy) FollowOAuth2Redirect(t *testing.T, redirectURL string) string {
	t.Helper()

	resp := h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, redirectURL, nil, false, http.StatusFound, nil)
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc, "OAuth2 redirect response should include Location")
	return loc
}

func (h *RemoteAuthProxy) DeliverOAuth2Callback(t *testing.T, callbackURL string) string {
	t.Helper()

	resp := h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodGet, callbackURL, nil, false, http.StatusFound, nil)
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc, "OAuth2 callback response should include Location")
	return loc
}

func (h *RemoteAuthProxy) DoProxyRequest(t *testing.T, connectionID, targetURL, method string) schemaapi.ProxyResponseJson {
	t.Helper()

	var proxyResp schemaapi.ProxyResponseJson
	h.doSigned(t, h.UserActorExternalID, h.UserActorNamespace, http.MethodPost, h.PublicURL+"/api/v1/connections/"+connectionID+"/_proxy", schemaapi.ProxyRequestJson{
		URL:    targetURL,
		Method: method,
	}, true, http.StatusOK, &proxyResp)
	return proxyResp
}

func (h *RemoteAuthProxy) doSigned(t *testing.T, actorExternalID, actorNamespace, method, rawURL string, body any, followRedirects bool, wantStatus int, out any) remoteResponse {
	t.Helper()
	return h.doSignedAllowing(t, actorExternalID, actorNamespace, method, rawURL, body, followRedirects, []int{wantStatus}, out)
}

func (h *RemoteAuthProxy) doSignedAllowing(t *testing.T, actorExternalID, actorNamespace, method, rawURL string, body any, followRedirects bool, wantStatuses []int, out any) remoteResponse {
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

	signer, err := h.signer(actorExternalID, actorNamespace)
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
	require.Containsf(t, wantStatuses, resp.StatusCode, "%s %s returned %d: %s", method, rawURL, resp.StatusCode, string(respBody))

	if out != nil {
		require.NoError(t, json.Unmarshal(respBody, out), "decode response from %s %s: %s", method, rawURL, string(respBody))
	}

	return remoteResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}
}

func (h *RemoteAuthProxy) signer(actorExternalID, actorNamespace string) (jwt.Signer, error) {
	builder := jwt.NewJwtTokenBuilder().
		WithActorExternalId(actorExternalID).
		WithNamespace(actorNamespace).
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
