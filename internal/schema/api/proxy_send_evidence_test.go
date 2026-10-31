package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestProxySendEvidenceContract requires evidence on current proxy envelopes
// while keeping the shared error contract usable by unrelated API endpoints.
func TestProxySendEvidenceContract(t *testing.T) {
	proxySchema := compileRefSchema(t, "./schema.json#/$defs/ProxyResponse")
	errorSchema := compileRefSchema(t, "./schema.json#/$defs/ErrorResponse")
	for _, evidence := range []common.SendEvidence{
		common.SendEvidenceNotSent, common.SendEvidenceMayHaveSent, common.SendEvidenceSent,
	} {
		t.Run(string(evidence), func(t *testing.T) {
			// Provider status and the outer error shape do not restrict the evidence.
			for _, contract := range []any{
				ProxyResponseJson{StatusCode: 429, SendEvidence: evidence},
				ErrorResponse{Error: "proxy failed", SendEvidence: evidence},
			} {
				encoded, err := json.Marshal(contract)
				require.NoError(t, err)
				var document map[string]any
				require.NoError(t, json.Unmarshal(encoded, &document))
				require.Equal(t, string(evidence), document["sendEvidence"])
				if _, isProxy := contract.(ProxyResponseJson); isProxy {
					require.NoError(t, proxySchema.Validate(document))
				} else {
					require.NoError(t, errorSchema.Validate(document))
				}

				// YAML uses the same camel-case transport field and enum values.
				asYAML, err := yaml.Marshal(contract)
				require.NoError(t, err)
				var yamlDocument map[string]any
				require.NoError(t, yaml.Unmarshal(asYAML, &yamlDocument))
				require.Equal(t, string(evidence), yamlDocument["sendEvidence"])
			}
		})
	}

	// Nonproxy errors retain their existing wire shape with no new metadata.
	encoded, err := json.Marshal(ErrorResponse{Error: "Bad Request"})
	require.NoError(t, err)
	require.JSONEq(t, `{"error":"Bad Request"}`, string(encoded))
	require.NoError(t, errorSchema.Validate(map[string]any{"error": "Bad Request"}))

	// New server envelopes require evidence; malformed present values always fail.
	require.Error(t, proxySchema.Validate(map[string]any{"statusCode": 200}))
	for _, invalid := range []any{"", "unknown", "not_sent", nil, false} {
		require.Error(t, proxySchema.Validate(map[string]any{"statusCode": 200, "sendEvidence": invalid}))
		require.Error(t, errorSchema.Validate(map[string]any{"error": "failed", "sendEvidence": invalid}))
	}
}
