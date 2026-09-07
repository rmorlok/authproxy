package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func connectorActionReference(generation uint64) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       connectorschema.ConnectorKind,
		ID:         apid.New(apid.PrefixConnector).String(),
		Generation: generation,
	}
}

func TestConnectorLifecycleActionValidation(t *testing.T) {
	request := NewConnectorLifecycleRequest(
		ConnectorDisconnectAllActionKind,
		connectorActionReference(0),
		ConnectorLifecycleSpec{},
	)
	require.NoError(t, request.ValidateRequest(ConnectorDisconnectAllActionKind))

	invalidTimeout := int64(0)
	request.Spec.TimeoutSeconds = &invalidTimeout
	require.ErrorContains(t, request.ValidateRequest(ConnectorDisconnectAllActionKind), "greater than zero")

	request.Spec.TimeoutSeconds = nil
	request.Metadata.Target.Generation = 1
	require.ErrorContains(t, request.ValidateRequest(ConnectorDisconnectAllActionKind), "does not apply")
}

func TestConnectorLifecycleResponseSerialization(t *testing.T) {
	response := NewConnectorLifecycleResponse(
		ConnectorArchiveActionKind,
		connectorActionReference(0),
		ConnectorLifecycleSpec{},
		"encrypted-task-info",
	)
	require.NoError(t, response.ValidateResponse(ConnectorArchiveActionKind))

	data, err := json.Marshal(response)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectorArchive",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":"`+response.Metadata.Target.ID+`"}},
      "spec":{},
      "status":{"taskId":"encrypted-task-info"}
    }`, string(data))

	invalidTimeout := int64(0)
	response.Spec.TimeoutSeconds = &invalidTimeout
	require.ErrorContains(t, response.ValidateResponse(ConnectorArchiveActionKind), "greater than zero")
}

func TestConnectorForceStateActionRequiresGeneration(t *testing.T) {
	request := NewConnectorForceStateRequest(
		connectorActionReference(2),
		connectorschema.ConnectorReleaseStatePrimary,
	)
	require.NoError(t, request.ValidateRequest(ConnectorForceStateActionKind))

	request.Metadata.Target.Generation = 0
	require.ErrorContains(t, request.ValidateRequest(ConnectorForceStateActionKind), "generation")
}
