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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (r *connectorDisconnectAllRig) archive(t *testing.T, connectorID apid.ID, timeoutSeconds int64) {
	t.Helper()

	reqBody, err := json.Marshal(schemaapi.ConnectorLifecycleRequestJson{
		TimeoutSeconds: int64Ptr(timeoutSeconds),
	})
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

	var body schemaapi.ConnectorLifecycleResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, connectorID, body.ConnectorId)
	require.NotEmpty(t, body.TaskId)

	requireWorkflowTaskCompleted(t, r.env, body.TaskId, "test-actor", time.Duration(timeoutSeconds+5)*time.Second)
}

func createArchiveVersionShape(t *testing.T, rig *connectorDisconnectAllRig, connector connectorDisconnectAllConnector) []uint64 {
	t.Helper()

	ctx := context.Background()
	draft, err := rig.env.Core.CreateDraftConnectorVersion(ctx, connector.id, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(2), draft.GetVersion())
	require.NoError(t, draft.SetState(ctx, database.ConnectorVersionStatePrimary))

	nextDraft, err := rig.env.Core.CreateDraftConnectorVersion(ctx, connector.id, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(3), nextDraft.GetVersion())

	requireConnectorVersionState(t, rig.env.Db, connector.id, 1, database.ConnectorVersionStateActive)
	requireConnectorVersionState(t, rig.env.Db, connector.id, 2, database.ConnectorVersionStatePrimary)
	requireConnectorVersionState(t, rig.env.Db, connector.id, 3, database.ConnectorVersionStateDraft)

	return []uint64{1, 2, 3}
}

func requireConnectorVersionState(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	version uint64,
	expected database.ConnectorVersionState,
) {
	t.Helper()

	connectorVersion, err := db.GetConnectorVersion(context.Background(), connectorID, version)
	require.NoError(t, err)
	require.Equal(t, expected, connectorVersion.State)
}

func requireConnectorVersionsArchived(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	versions []uint64,
) {
	t.Helper()

	for _, version := range versions {
		requireConnectorVersionState(t, db, connectorID, version, database.ConnectorVersionStateArchived)
	}
}

func TestConnectorArchive_ArchivesVersionsAndDisconnectsConnections(t *testing.T) {
	rig := newConnectorDisconnectAllRig(t, "connector-archive", 1)

	connectionID := rig.completeAuthFlow(t, rig.connectors[0])
	requireConnectionAvailable(t, rig, connectionID)
	versions := createArchiveVersionShape(t, rig, rig.connectors[0])

	startCoreWorkflowWorker(t, rig.env)
	rig.archive(t, rig.connectors[0].id, 20)

	requireConnectorVersionsArchived(t, rig.env.Db, rig.connectors[0].id, versions)
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
	versions := createArchiveVersionShape(t, rig, rig.connectors[0])

	rig.provider.Script(rig.connectors[0].clientKey, helpers.EndpointRevoke, helpers.ScriptAction{
		Status:    http.StatusServiceUnavailable,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	startCoreWorkflowWorker(t, rig.env)
	rig.archive(t, rig.connectors[0].id, 20)

	requireConnectorVersionsArchived(t, rig.env.Db, rig.connectors[0].id, versions)
	requireConnectionDeletedByID(t, rig.env, connectionID)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, connectionID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[0].clientKey,
	})
	assert.Lenf(t, revokeReqs, 3,
		"archive should allow child disconnect to exhaust revocation retries, then force local disconnect before final archival")
}
