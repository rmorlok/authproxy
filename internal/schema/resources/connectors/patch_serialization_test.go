package connectors

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConnectorSpecPatchDecoding preserves nested release presence and secret
// values while replacing reused receivers only after a successful decode.
func TestConnectorSpecPatchDecoding(t *testing.T) {
	definition := testConnectorResource().Spec.Definition
	populated, err := json.Marshal(ConnectorSpecPatch{Definition: &definition})
	require.NoError(t, err)
	for _, format := range []struct {
		name   string
		decode func([]byte, any) error
		encode func(any) ([]byte, error)
	}{
		{"JSON", json.Unmarshal, json.Marshal},
		{"YAML", yaml.Unmarshal, yaml.Marshal},
	} {
		t.Run(format.name, func(t *testing.T) {
			for _, input := range []string{
				`{}`, `{"release":null,"definition":null}`,
				`{"release":{}}`, `{"release":{"desiredState":null}}`,
				`{"release":{"desiredState":"primary"}}`, string(populated),
			} {
				var patch ConnectorSpecPatch
				require.NoError(t, format.decode([]byte(input), &patch))
				// Null presence is checked via JSON below; YAML round trips use
				// non-null values supported by the existing nested marshalers.
				if !patch.HasRelease() || patch.Release != nil {
					encoded, err := format.encode(patch)
					require.NoError(t, err)
					var roundtrip ConnectorSpecPatch
					require.NoError(t, format.decode(encoded, &roundtrip))
					require.Equal(t, patch, roundtrip)
				}
				wire, err := json.Marshal(patch)
				require.NoError(t, err)
				require.JSONEq(t, input, string(wire))

				require.NoError(t, format.decode([]byte(`{}`), &patch))
				require.Equal(t, ConnectorSpecPatch{}, patch)
			}
			for _, input := range []string{
				`{"release":{"desiredState":"draft"},"unknown":true}`,
				`{"release":{"desiredState":"draft","unknown":true}}`,
				`{"definition":{"displayName":"updated","unknown":true}}`,
				`{"release":{"desiredState":[]}}`,
			} {
				var patch, before ConnectorSpecPatch
				require.NoError(t, format.decode(populated, &patch))
				require.NoError(t, format.decode(populated, &before))
				require.Error(t, format.decode([]byte(input), &patch))
				require.Equal(t, before, patch)
			}

			// Decode release patches directly too: the nested pointer path alone
			// cannot detect stale state retained by a reused release receiver.
			for _, seed := range []string{`{"desiredState":null}`, `{"desiredState":"primary"}`} {
				var release, before ConnectorReleaseSpecPatch
				require.NoError(t, format.decode([]byte(seed), &release))
				require.NoError(t, format.decode([]byte(seed), &before))
				require.True(t, release.HasDesiredState())
				require.Error(t, format.decode([]byte(`{"desiredState":"draft","unknown":true}`), &release))
				require.Equal(t, before, release)
				require.NoError(t, format.decode([]byte(`{}`), &release))
				require.Equal(t, ConnectorReleaseSpecPatch{}, release)
			}
		})
	}
}
