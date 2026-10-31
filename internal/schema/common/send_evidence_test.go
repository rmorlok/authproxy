package common

import (
	"encoding/json"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestSendEvidenceSchema checks the shared wire vocabulary against its Go
// validation and both codecs, including conservative rejection of absent data.
func TestSendEvidenceSchema(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	id := loadSchema(t, compiler, "schema.json")
	schema, err := compiler.Compile(id + "#/$defs/SendEvidence")
	require.NoError(t, err)

	for _, evidence := range []SendEvidence{SendEvidenceNotSent, SendEvidenceMayHaveSent, SendEvidenceSent} {
		t.Run(string(evidence), func(t *testing.T) {
			// Every supported value has identical meaning in JSON, YAML, and schema.
			require.NoError(t, evidence.Validate())
			require.NoError(t, schema.Validate(string(evidence)))
			encoded, err := json.Marshal(evidence)
			require.NoError(t, err)
			var decoded SendEvidence
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, evidence, decoded)
			asYAML, err := yaml.Marshal(evidence)
			require.NoError(t, err)
			require.NoError(t, yaml.Unmarshal(asYAML, &decoded))
			require.Equal(t, evidence, decoded)
		})
	}

	// Zero values and future/unknown vocabulary cannot masquerade as notSent.
	for _, value := range []string{"", "unknown", "not_sent", "SENT"} {
		require.Error(t, SendEvidence(value).Validate())
		require.Error(t, schema.Validate(value))
	}
	for _, value := range []any{nil, false, float64(0)} {
		require.Error(t, schema.Validate(value))
	}
}
