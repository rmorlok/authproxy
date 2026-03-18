package pagination

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type FakeCursor struct {
	Value string `json:"value"`
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()
	enc := NewDefaultCursorEncryptor([]byte("0123456789abcdef0123456789abcdef"))

	cursor, err := MakeCursor(context.Background(), enc, &FakeCursor{
		Value: "some-value",
	})
	require.NoError(t, err)
	require.NotEmpty(t, cursor)

	parsed, err := ParseCursor[FakeCursor](context.Background(), enc, cursor)
	require.NoError(t, err)
	require.Equal(t, "some-value", parsed.Value)
}
