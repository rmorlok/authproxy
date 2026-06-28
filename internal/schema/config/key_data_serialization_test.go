package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKeyDataRandomBytesJSONRoundTrip(t *testing.T) {
	var kd KeyData
	require.NoError(t, json.Unmarshal([]byte(`{"num_bytes":32}`), &kd))

	randomBytes, ok := kd.InnerVal.(*KeyDataRandomBytes)
	require.True(t, ok)
	require.Equal(t, 32, randomBytes.NumBytes)

	data, err := json.Marshal(&kd)
	require.NoError(t, err)

	var roundTrip KeyData
	require.NoError(t, json.Unmarshal(data, &roundTrip))

	roundTripRandomBytes, ok := roundTrip.InnerVal.(*KeyDataRandomBytes)
	require.True(t, ok)
	require.Equal(t, 32, roundTripRandomBytes.NumBytes)
}

func TestKeyDataRandomBytesYAMLAcceptsNumBytes(t *testing.T) {
	var kd KeyData
	require.NoError(t, yaml.Unmarshal([]byte("num_bytes: 32\n"), &kd))

	randomBytes, ok := kd.InnerVal.(*KeyDataRandomBytes)
	require.True(t, ok)
	require.Equal(t, 32, randomBytes.NumBytes)
}
