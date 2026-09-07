package helpers

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

const defaultConnectionReturnToURL = "https://app.example.test/connections"

func connectorReference(id apid.ID, generation uint64) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       connectorschema.ConnectorKind,
		ID:         id.String(),
		Generation: generation,
	}
}

func connectionReference(id string) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       connectionschema.ConnectionKind,
		ID:         id,
	}
}

func connectionInitiateAction(
	connectorID apid.ID,
	intoNamespace string,
	returnToURL string,
) schemaapi.ConnectionInitiateAction {
	if returnToURL == "" {
		returnToURL = defaultConnectionReturnToURL
	}
	return schemaapi.ConnectionInitiateAction{Action: apiv1alpha1.NewActionRequest(
		schemaapi.ConnectionInitiateActionKind,
		connectorReference(connectorID, 0),
		schemaapi.ConnectionInitiateSpec{
			IntoNamespace: intoNamespace,
			ReturnToURL:   returnToURL,
		},
	)}
}

func connectionSetupSubmitAction(
	connectionID string,
	stepID string,
	data json.RawMessage,
) schemaapi.ConnectionSetupSubmitAction {
	return schemaapi.ConnectionSetupSubmitAction{Action: apiv1alpha1.NewActionRequest(
		schemaapi.ConnectionSetupSubmitActionKind,
		connectionReference(connectionID),
		schemaapi.ConnectionSetupSubmitSpec{StepID: stepID, Data: data},
	)}
}

func connectionSetupControlAction(
	kind meta.Kind,
	connectionID string,
	returnToURL string,
) schemaapi.ConnectionSetupControlAction {
	return schemaapi.ConnectionSetupControlAction{Action: apiv1alpha1.NewActionRequest(
		kind,
		connectionReference(connectionID),
		schemaapi.ConnectionSetupControlSpec{ReturnToURL: returnToURL},
	)}
}

func decodeConnectionSetupAction(
	t *testing.T,
	data []byte,
) schemaapi.ConnectionSetupAction {
	t.Helper()

	var action schemaapi.ConnectionSetupAction
	require.NoError(t, json.Unmarshal(data, &action))
	require.NoError(t, action.ValidateResponse(schemaapi.ConnectionSetupActionKind),
		"invalid setup response: %s", string(data))
	return action
}
