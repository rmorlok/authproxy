//go:build integration

package api_key

import (
	"context"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApiKeyManualRotation drives the user-initiated rotation path: a Ready
// connection invokes POST /_reauth, submits a new credential, and the prior
// row is soft-deleted in the same transaction (verified via the database
// state). The no-replay invariant is asserted at every observable surface —
// the reauth form payload, the credential schema, and the post-rotation
// upstream request stream all must omit the prior key bytes.
func TestApiKeyManualRotation(t *testing.T) {
	const oldKey = "manual-rotation-old-key"
	const newKey = "manual-rotation-new-key"

	stub := helpers.NewApiKeyStubUpstream(t, helpers.ApiKeyStubOptions{
		Placement:   connectors.ApiKeyPlacementBearer,
		AcceptedKey: oldKey,
	})

	connectorID := apid.New(apid.PrefixConnectorVersion)
	conn := helpers.NewApiKeyConnector(connectorID, "api-key-rotation", helpers.ApiKeyConnectorOptions{
		Placement: connectors.ApiKeyPlacementBearer,
		ProbeURL:  stub.BaseURL + "/probe",
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:    helpers.ServiceTypeAPI,
		Connectors: []sconfig.Connector{conn},
	})
	defer env.Cleanup()

	// 1. Drive to Ready with the old key.
	connectionID, form := env.InitiateApiKeyConnection(t, connectorID)
	w := env.SubmitApiKeyCredentials(t, connectionID, form.StepId, map[string]any{
		"api_key": oldKey,
	})
	require.Equalf(t, http.StatusOK, w.Code, "submit failed: %s", w.Body.String())
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	cReady := env.GetConnection(t, connectionID)
	require.Equal(t, database.ConnectionStateConfigured, cReady.State)

	// Snapshot the active credential row before rotation so we can confirm
	// the row id changes (i.e. a new row was inserted, not the old one
	// updated in place).
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	credBefore, err := env.Db.GetActiveApiKeyCredential(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, oldKey, env.DecryptApiKeyCredential(t, connectionID).ApiKey,
		"sanity: pre-rotation active credential should match what we submitted")

	// Rotate the upstream's accepted key so the OLD credential would now
	// fail a probe — this is what would also drive auto-reauth in real
	// usage. We don't run a probe here yet; that's the auto_reauth test.
	stub.RotateAcceptedKey(newKey)

	// Count upstream requests before reauth so we can scan only the
	// requests that arrive *after* rotation begins for the no-replay
	// assertion (pre-rotation requests obviously contain the old key —
	// that's the whole point of them being authenticated).
	requestsBeforeReauth := len(stub.Requests())

	// 2. POST /_reauth — the response must be a form (credentials step)
	//    and must NOT contain the prior key bytes.
	w = env.ReauthConnection(t, connectionID)
	require.Equalf(t, http.StatusOK, w.Code, "reauth failed: %s", w.Body.String())

	rawBody := w.Body.String()
	assert.NotContainsf(t, rawBody, oldKey,
		"reauth form payload must not echo back the prior key bytes")

	// 3. Submit the new credential. The submit handler runs InsertApiKey-
	//    Credential, which soft-deletes the prior row in the same tx —
	//    so the active row id changes and the prior row's deleted_at is
	//    populated.
	w = env.SubmitApiKeyCredentials(t, connectionID, helpers.ApiKeySubmitFormStepId(), map[string]any{
		"api_key": newKey,
	})
	require.Equalf(t, http.StatusOK, w.Code, "rotation submit failed: %s", w.Body.String())

	// 4. Verify against the rotated upstream succeeds with the new key
	//    and the connection lands in Ready / healthy.
	require.NoError(t, env.RunVerifyConnection(t, connectionID))

	cAfter := env.GetConnection(t, connectionID)
	require.Equal(t, database.ConnectionStateConfigured, cAfter.State)
	assert.Equal(t, database.ConnectionHealthStateHealthy, cAfter.HealthState)

	// 5. Active credential row was replaced — different id, different
	//    plaintext.
	credAfter, err := env.Db.GetActiveApiKeyCredential(context.Background(), id)
	require.NoError(t, err)
	assert.NotEqualf(t, credBefore.Id, credAfter.Id,
		"rotation must produce a new credential row (got same id)")
	assert.Equal(t, newKey, env.DecryptApiKeyCredential(t, connectionID).ApiKey,
		"post-rotation active credential should be the new key")

	// 6. No-replay invariant: no request that arrived after the reauth
	//    began (i.e. after the upstream-side rotation) should carry the
	//    old key in any header or query parameter. The pre-rotation
	//    requests carry the old key by design — we exclude them by
	//    slicing.
	postRequests := stub.Requests()[requestsBeforeReauth:]
	found, offending := helpers.ContainsBytes(postRequests, oldKey)
	assert.Falsef(t, found,
		"old key %q must not appear in any upstream request after rotation; offending request=%+v",
		oldKey, offending)
}
