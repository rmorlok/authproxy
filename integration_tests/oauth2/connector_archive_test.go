//go:build integration

package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (r *connectorDisconnectAllRig) archive(t *testing.T, connectorID apid.ID, timeoutSeconds int64) {
	t.Helper()

	reqBody, err := json.Marshal(schemaapi.NewConnectorLifecycleRequest(
		schemaapi.ConnectorArchiveActionKind,
		meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       cschema.ConnectorKind,
			ID:         connectorID.String(),
		},
		schemaapi.ConnectorLifecycleSpec{TimeoutSeconds: int64Ptr(timeoutSeconds)},
	))
	require.NoError(t, err)

	path := "/api/v1/connectors/" + connectorID.String() + "/_archive"
	req, err := r.env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		path,
		bytes.NewReader(reqBody),
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.env.ApiGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusOK, w.Code, "archive failed: %s", w.Body.String())

	var body schemaapi.ConnectorLifecycleAction
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NoError(t, body.ValidateResponse(schemaapi.ConnectorArchiveActionKind))
	require.Equal(t, connectorID.String(), body.Metadata.Target.ID)
	require.NotNil(t, body.Status)
	require.NotEmpty(t, body.Status.TaskID)

	helpers.RequireWorkflowTaskCompleted(t, r.env, body.Status.TaskID, time.Duration(timeoutSeconds+5)*time.Second)
}

func createArchiveGenerationShape(t *testing.T, rig *connectorDisconnectAllRig, connector connectorDisconnectAllConnector) []uint64 {
	t.Helper()

	ctx := context.Background()
	draft, err := rig.env.Core.CreateDraftConnectorGeneration(ctx, connector.id, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(2), draft.GetGeneration())
	require.NoError(t, draft.SetState(ctx, database.ConnectorGenerationStatePrimary))

	nextDraft, err := rig.env.Core.CreateDraftConnectorGeneration(ctx, connector.id, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(3), nextDraft.GetGeneration())

	requireConnectorGenerationState(t, rig.env.Db, connector.id, 1, database.ConnectorGenerationStateActive)
	requireConnectorGenerationState(t, rig.env.Db, connector.id, 2, database.ConnectorGenerationStatePrimary)
	requireConnectorGenerationState(t, rig.env.Db, connector.id, 3, database.ConnectorGenerationStateDraft)

	return []uint64{1, 2, 3}
}

func requireConnectorGenerationState(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	generation uint64,
	expected database.ConnectorGenerationState,
) {
	t.Helper()

	connectorGeneration, err := db.GetConnectorGeneration(context.Background(), connectorID, generation)
	require.NoError(t, err)
	require.Equal(t, expected, connectorGeneration.State)
}

func requireConnectorGenerationsArchived(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	generations []uint64,
) {
	t.Helper()

	for _, generation := range generations {
		requireConnectorGenerationState(t, db, connectorID, generation, database.ConnectorGenerationStateArchived)
	}
}

func TestConnectorArchive_ArchivesGenerationsAndDisconnectsConnections(t *testing.T) {
	rig := newConnectorDisconnectAllRig(t, "connector-archive", 1)

	connectionID := rig.completeAuthFlow(t, rig.connectors[0])
	requireConnectionAvailable(t, rig, connectionID)
	generations := createArchiveGenerationShape(t, rig, rig.connectors[0])

	helpers.StartCoreWorkflowWorker(t, rig.env)
	rig.archive(t, rig.connectors[0].id, 20)

	requireConnectorGenerationsArchived(t, rig.env.Db, rig.connectors[0].id, generations)
	requireConnectionDeletedByID(t, rig.env, connectionID)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, connectionID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[0].clientKey,
	})
	assert.Len(t, revokeReqs, 1)
}

func TestConnectorArchive_RevocationFailureStillArchives(t *testing.T) {
	rig := newConnectorDisconnectAllRig(t, "connector-archive-fail", 1)

	connectionID := rig.completeAuthFlow(t, rig.connectors[0])
	require.NotNil(t, rig.env.GetOAuth2Token(t, connectionID))
	generations := createArchiveGenerationShape(t, rig, rig.connectors[0])

	rig.provider.Script(rig.connectors[0].clientKey, helpers.EndpointRevoke, helpers.ScriptAction{
		Status:    http.StatusServiceUnavailable,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	helpers.StartCoreWorkflowWorker(t, rig.env)
	rig.archive(t, rig.connectors[0].id, 20)

	requireConnectorGenerationsArchived(t, rig.env.Db, rig.connectors[0].id, generations)
	requireConnectionDeletedByID(t, rig.env, connectionID)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, connectionID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[0].clientKey,
	})
	assert.Lenf(t, revokeReqs, 3,
		"archive should allow child disconnect to exhaust revocation retries, then force local disconnect before final archival")
}
