package helpers

import (
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/stretchr/testify/require"
)

// InitiateNoAuthConnection creates a no-auth connection through the public
// setup API. No-auth connectors have no credentials or setup steps, so a
// successful initiation completes the connection immediately.
func (env *IntegrationTestEnv) InitiateNoAuthConnection(
	t *testing.T,
	connectorID apid.ID,
	opts ...OAuth2Option,
) string {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "",
		"InitiateNoAuthConnection requires either in-process gin or a running HTTP server")

	cfg := env.resolveOAuth2Options(opts)
	body, err := jsonMarshal(coreIface.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		IntoNamespace: cfg.actorNamespace,
	})
	require.NoError(t, err)

	w := env.doSignedRequest(t, http.MethodPost, "/api/v1/connections/_initiate", body, cfg)
	require.Equalf(t, http.StatusOK, w.Code, "initiate failed: %s", w.Body.String())

	var response coreIface.ConnectionSetupComplete
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &response))
	require.Equal(t, coreIface.ConnectionSetupResponseTypeComplete, response.Type,
		"expected no-auth connection to complete immediately: %s", w.Body.String())
	require.NotEqual(t, apid.Nil, response.Id)
	return response.Id.String()
}
