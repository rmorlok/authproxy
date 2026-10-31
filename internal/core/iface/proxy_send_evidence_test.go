package iface

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

// TestProxyResponseSendEvidenceSchema checks the real serialized runtime
// envelope, including its existing null body fields, against the public schema.
func TestProxyResponseSendEvidenceSchema(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	var apiSchemaID string
	for _, path := range []string{"../../schema/common/schema.json", "../../schema/api/schema.json"} {
		encoded, err := os.ReadFile(path)
		require.NoError(t, err)
		var schemaHeader struct {
			ID string `json:"$id"`
		}
		require.NoError(t, json.Unmarshal(encoded, &schemaHeader))
		require.NoError(t, compiler.AddResource(schemaHeader.ID, bytes.NewReader(encoded)))
		apiSchemaID = schemaHeader.ID
	}
	schema, err := compiler.Compile(apiSchemaID + "#/$defs/ProxyResponse")
	require.NoError(t, err)

	for _, response := range []ProxyResponse{
		{StatusCode: 200, Headers: map[string]string{}, BodyJson: map[string]any{"ok": true}, SendEvidence: common.SendEvidenceSent},
		{StatusCode: 200, Headers: map[string]string{}, BodyRaw: []byte("plain text"), SendEvidence: common.SendEvidenceSent},
		{StatusCode: 429, Headers: map[string]string{}, SendEvidence: common.SendEvidenceNotSent},
		{StatusCode: 200, Headers: map[string]string{}, SendEvidence: common.SendEvidenceMayHaveSent},
	} {
		// Metadata is additive; status, headers, and both body keys remain present.
		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		var document map[string]any
		require.NoError(t, json.Unmarshal(encoded, &document))
		require.NoError(t, schema.Validate(document))
		require.Equal(t, float64(response.StatusCode), document["statusCode"])
		require.Contains(t, document, "headers")
		require.Contains(t, document, "bodyRaw")
		require.Contains(t, document, "bodyJson")
		require.Equal(t, string(response.SendEvidence), document["sendEvidence"])
		if response.BodyRaw == nil {
			require.Nil(t, document["bodyRaw"])
		}
		if response.BodyJson == nil {
			require.Nil(t, document["bodyJson"])
		}
	}
}
