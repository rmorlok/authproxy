package helpers

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/require"
)

func TestConnectionActionFixturesUseV1Alpha1Envelopes(t *testing.T) {
	connectorID := apid.MustParse("cxr_testconnector0001")
	initiate := connectionInitiateAction(connectorID, "root.tenant", "https://app.example.test/return")
	require.NoError(t, initiate.ValidateRequest(schemaapi.ConnectionInitiateActionKind))

	data, err := json.Marshal(initiate)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionInitiate",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":"cxr_testconnector0001"}},
      "spec":{"intoNamespace":"root.tenant","returnToUrl":"https://app.example.test/return"}
    }`, string(data))

	withoutExplicitReturn := connectionInitiateAction(connectorID, "root.tenant", "")
	require.NoError(t, withoutExplicitReturn.ValidateRequest(schemaapi.ConnectionInitiateActionKind))
	data, err = json.Marshal(withoutExplicitReturn)
	require.NoError(t, err)
	require.Contains(t, string(data), `"returnToUrl":"https://app.example.test/connections"`)

	submit := connectionSetupSubmitAction("cxn_testconnection01", "credentials", json.RawMessage(`{"apiKey":"test"}`))
	require.NoError(t, submit.ValidateRequest(schemaapi.ConnectionSetupSubmitActionKind))
	data, err = json.Marshal(submit)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind":"ConnectionSetupSubmit"`)
	require.Contains(t, string(data), `"kind":"Connection"`)
	require.Contains(t, string(data), `"stepId":"credentials"`)

	reauth := connectionSetupControlAction(schemaapi.ConnectionReauthActionKind, "cxn_testconnection01", "https://app.example.test/return")
	require.NoError(t, reauth.ValidateRequest(schemaapi.ConnectionReauthActionKind))
	data, err = json.Marshal(reauth)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind":"ConnectionReauthenticate"`)
	require.Contains(t, string(data), `"returnToUrl":"https://app.example.test/return"`)
}
