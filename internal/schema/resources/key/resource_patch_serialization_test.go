package key

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestKeySpecPatchDecoding preserves write-only provider material and explicit
// null presence without retaining old fields or mutating a receiver on failure.
func TestKeySpecPatchDecoding(t *testing.T) {
	const populated = `{"usage":"data_encryption","materialType":"symmetric","desiredState":"active","keyData":{"value":"super-secret-key"}}`
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
		encode func(any) ([]byte, error)
	}{
		{"JSON", json.Unmarshal, json.Marshal},
		{"YAML", yaml.Unmarshal, yaml.Marshal},
	} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{`{}`, `{"keyData":null}`, populated} {
				var patch KeySpecPatch
				require.NoError(t, format.decode([]byte(input), &patch))
				require.Equal(t, input != `{}`, patch.HasKeyData())
				if patch.KeyData != nil {
					require.Equal(t, "super-secret-key", patch.KeyData.InnerVal.(*KeyDataValue).Value)
					redacted, err := RedactKeyData(patch.KeyData)
					require.NoError(t, err)
					require.NotEqual(t, "super-secret-key", redacted.InnerVal.(*KeyDataValue).Value)
					require.Equal(t, "super-secret-key", patch.KeyData.InnerVal.(*KeyDataValue).Value)
				}
				// Null presence is checked via JSON below; YAML round trips use
				// non-null values supported by the existing nested marshalers.
				if !patch.HasKeyData() || patch.KeyData != nil {
					encoded, err := format.encode(patch)
					require.NoError(t, err)
					var roundtrip KeySpecPatch
					require.NoError(t, format.decode(encoded, &roundtrip))
					require.Equal(t, patch, roundtrip)
				}
				wire, err := json.Marshal(patch)
				require.NoError(t, err)
				require.JSONEq(t, input, string(wire))

				require.NoError(t, format.decode([]byte(`{}`), &patch))
				require.Equal(t, KeySpecPatch{}, patch)
			}
			for _, input := range []string{
				`{"desiredState":"disabled","unknown":true}`,
				`{"keyData":{"value":"replacement","unknown":true}}`,
				`{"desiredState":"disabled","keyData":[]}`,
			} {
				var patch, before KeySpecPatch
				require.NoError(t, format.decode([]byte(populated), &patch))
				require.NoError(t, format.decode([]byte(populated), &before))
				require.Error(t, format.decode([]byte(input), &patch))
				require.Equal(t, before, patch)
			}
		})
	}
}
