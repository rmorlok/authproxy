package helpers

import (
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/require"
)

// InitiateNoAuthConnection creates a no-auth connection through the public
// setup API. No-auth connectors have no credentials or setup steps, so a
// successful initiation completes the connection immediately.
func (env *IntegrationTestEnv) InitiateNoAuthConnection(
	t *testing.T,
	connectorID apid.ID,
	opts ...ActorOption,
) string {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "",
		"InitiateNoAuthConnection requires either in-process gin or a running HTTP server")

	cfg := env.resolveActorOptions(opts)
	body, err := jsonMarshal(connectionInitiateAction(connectorID, cfg.actorNamespace, ""))
	require.NoError(t, err)

	w := env.doSignedRequest(t, http.MethodPost, "/api/v1/connections/_initiate", body, cfg)
	require.Equalf(t, http.StatusOK, w.Code, "initiate failed: %s", w.Body.String())

	response := decodeConnectionSetupAction(t, w.Body.Bytes())
	require.Equal(t, schemaapi.ConnectionSetupResponseTypeComplete, response.Status.Type,
		"expected no-auth connection to complete immediately: %s", w.Body.String())
	require.NotEmpty(t, response.Metadata.Target.ID)
	return response.Metadata.Target.ID
}
