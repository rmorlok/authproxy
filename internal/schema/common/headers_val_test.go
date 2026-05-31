package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeadersValJSON(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var got HeadersVal
		require.NoError(t, json.Unmarshal([]byte(`"application/json"`), &got))
		require.Equal(t, []string{"application/json"}, got.Values())

		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		require.JSONEq(t, `"application/json"`, string(encoded))
	})

	t.Run("array", func(t *testing.T) {
		var got HeadersVal
		require.NoError(t, json.Unmarshal([]byte(`["a=1", "b=2"]`), &got))
		require.Equal(t, []string{"a=1", "b=2"}, got.Values())

		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		require.JSONEq(t, `["a=1", "b=2"]`, string(encoded))
	})

	t.Run("rejects non-string values", func(t *testing.T) {
		var got HeadersVal
		require.Error(t, json.Unmarshal([]byte(`["ok", 7]`), &got))
		require.Error(t, json.Unmarshal([]byte(`{"value":"ok"}`), &got))
	})
}
