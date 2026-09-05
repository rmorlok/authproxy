package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func connectionActionTestReference(kind meta.Kind, id apid.ID, generation uint64) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       kind,
		ID:         id.String(),
		Generation: generation,
	}
}

func connectionActionTestResource(t *testing.T) connectionschema.Connection {
	t.Helper()
	now := time.Now().UTC()
	return connectionschema.Connection{
		TypeMeta: meta.NewTypeMeta(connectionschema.ConnectionKind),
		Metadata: meta.ObjectMeta{
			ID:        apid.New(apid.PrefixConnection).String(),
			Name:      "production",
			Namespace: "root.acme",
			CreatedAt: &now,
			UpdatedAt: &now,
		},
		Spec: connectionschema.ConnectionSpec{ConnectorRef: connectionActionTestReference(
			connectorschema.ConnectorKind,
			apid.New(apid.PrefixConnector),
			2,
		)},
		Status: &connectionschema.ConnectionStatus{
			Lifecycle:               connectionschema.ConnectionLifecycleStatus{State: connectionschema.ConnectionStateConfigured},
			Health:                  connectionschema.ConnectionHealthStatus{State: connectionschema.ConnectionHealthStateHealthy},
			ConfigurationConfigured: true,
		},
	}
}

func TestConnectionInitiateActionValidation(t *testing.T) {
	target := connectionActionTestReference(connectorschema.ConnectorKind, apid.New(apid.PrefixConnector), 0)
	action := ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
		ConnectionInitiateActionKind,
		target,
		ConnectionInitiateSpec{ReturnToURL: "https://app.example.com/complete"},
	)}
	require.NoError(t, action.ValidateRequest(ConnectionInitiateActionKind))

	action.Spec.ReturnToURL = ""
	require.ErrorContains(t, action.ValidateRequest(ConnectionInitiateActionKind), "returnToUrl")
	action.Spec.ReturnToURL = "https://app.example.com/complete"
	action.Metadata.Target.Kind = connectionschema.ConnectionKind
	require.ErrorContains(t, action.ValidateRequest(ConnectionInitiateActionKind), "Connector")
}

func TestConnectionSetupActionValidationAndRedaction(t *testing.T) {
	target := connectionschema.NewConnectionReference(apid.New(apid.PrefixConnection))
	action := NewConnectionSetupAction(target, ConnectionSetupActionStatus{
		Type:       ConnectionSetupResponseTypeForm,
		StepID:     "credentials",
		JSONSchema: json.RawMessage(`{"type":"object"}`),
		UISchema:   json.RawMessage(`{}`),
		Data:       json.RawMessage(`{"apiKey":"secret-value"}`),
	})
	require.NoError(t, action.ValidateResponse(ConnectionSetupActionKind))

	encoded, report, err := apserde.MarshalJSONForAPI(context.Background(), action)
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.NotContains(t, string(encoded), "secret-value")
	require.Contains(t, string(encoded), `"apiKey":"************"`)

	redacted, err := RedactConnectionSetupData(action.Status.Data)
	require.NoError(t, err)
	action.Status.Data = redacted
	encoded, report, err = apserde.MarshalJSONForAPI(
		apserde.WithSecretReplay(context.Background(), true),
		action,
	)
	require.NoError(t, err)
	require.False(t, report.Redacted, "setup data is already irreversibly redacted")
	require.NotContains(t, string(encoded), "secret-value")
	require.Contains(t, string(encoded), `"apiKey":"************"`)

	action.Status.StepID = ""
	require.ErrorContains(t, action.ValidateResponse(ConnectionSetupActionKind), "stepId")
}

func TestConnectionLifecycleActionResponseValidation(t *testing.T) {
	connection := connectionActionTestResource(t)
	target := connectionschema.NewConnectionReference(apid.MustParse(connection.Metadata.ID))
	timeout := int64(60)

	disconnect := NewConnectionDisconnectResponse(target, ConnectionDisconnectSpec{TimeoutSeconds: &timeout}, ConnectionDisconnectStatus{
		TaskID:     "task-token",
		Connection: connection,
	})
	require.NoError(t, disconnect.ValidateResponse(ConnectionDisconnectActionKind))
	disconnect.Status.TaskID = ""
	require.ErrorContains(t, disconnect.ValidateResponse(ConnectionDisconnectActionKind), "taskId")

	connectorRef := connection.Spec.ConnectorRef
	migration := NewConnectionVersionMigrationResponse(target, ConnectionVersionMigrationSpec{ConnectorRef: connectorRef}, ConnectionVersionMigrationStatus{
		TaskID:             "task-token",
		SourceConnectorRef: connectorRef,
		TargetConnectorRef: connectorRef,
	})
	require.NoError(t, migration.ValidateResponse(ConnectionVersionMigrationActionKind))
	migration.Status.TargetConnectorRef.Generation = 0
	require.ErrorContains(t, migration.ValidateResponse(ConnectionVersionMigrationActionKind), "generation")

	forceState := NewConnectionForceStateResponse(target, ConnectionForceStateSpec{
		State: connectionschema.ConnectionStateConfigured,
	}, connection)
	require.NoError(t, forceState.ValidateResponse(ConnectionForceStateActionKind))
	forceState.Status.Connection.Status = nil
	require.ErrorContains(t, forceState.ValidateResponse(ConnectionForceStateActionKind), "status")
}
