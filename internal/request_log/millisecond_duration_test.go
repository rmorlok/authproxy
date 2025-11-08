package request_log

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMillisecondDuration(t *testing.T) {
	t.Run("it roundtrips to json", func(t *testing.T) {
		orig := MillisecondDuration(123 * time.Second)
		data, err := json.Marshal(orig)
		require.NoError(t, err)

		var result MillisecondDuration
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		require.Equal(t, orig, result)
		require.Equal(t, orig.Duration(), result.Duration())
	})
	t.Run("it renders as milliseconds in json", func(t *testing.T) {
		orig := MillisecondDuration(1234 * time.Millisecond)
		data, err := json.Marshal(orig)
		require.NoError(t, err)
		require.Equal(t, []byte("1234"), data)
	})
	t.Run("it formats as a string", func(t *testing.T) {
		orig := MillisecondDuration(123 * time.Second)
		require.Equal(t, "123000", orig.String())
	})
}
