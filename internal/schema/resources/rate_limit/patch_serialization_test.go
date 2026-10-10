package rate_limit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRateLimitSpecPatchDecoding preserves presence, exact integers, and receiver
// ownership through both supported formats.
func TestRateLimitSpecPatchDecoding(t *testing.T) {
	const populated = `{"scope":{"namespaceMatcher":"root.acme.**"},"mode":"observe","selector":{"methods":["GET"]},"bucket":{"dimensions":["actor"]},"algorithm":{"tokenBucket":{"capacity":9007199254740993,"refillRate":0.5}}}`
	const nulls = `{"scope":null,"mode":null,"selector":null,"bucket":null,"algorithm":null}`
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
		encode func(any) ([]byte, error)
	}{
		{"JSON", json.Unmarshal, json.Marshal},
		{"YAML", yaml.Unmarshal, yaml.Marshal},
	} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{`{}`, nulls, populated} {
				var patch RateLimitSpecPatch
				require.NoError(t, format.decode([]byte(input), &patch))
				encoded, err := format.encode(patch)
				require.NoError(t, err)
				var roundtrip RateLimitSpecPatch
				require.NoError(t, format.decode(encoded, &roundtrip))
				require.Equal(t, patch, roundtrip)
				if patch.Algorithm != nil {
					require.Equal(t, int64(9007199254740993), int64(patch.Algorithm.TokenBucket.Capacity))
				}
				wire, err := json.Marshal(patch)
				require.NoError(t, err)
				require.JSONEq(t, input, string(wire))

				// Both values and explicit-null flags must disappear on reuse.
				require.NoError(t, format.decode([]byte(`{}`), &patch))
				require.Equal(t, RateLimitSpecPatch{}, patch)
			}
			for _, input := range []string{
				`{"mode":"enforce","unknown":true}`,
				`{"algorithm":{"tokenBucket":{"capacity":1,"unknown":true}}}`,
				`{"mode":"enforce","bucket":[]}`,
			} {
				var patch, before RateLimitSpecPatch
				require.NoError(t, format.decode([]byte(populated), &patch))
				require.NoError(t, format.decode([]byte(populated), &before))
				require.Error(t, format.decode([]byte(input), &patch))
				require.Equal(t, before, patch)
			}
		})
	}
}
