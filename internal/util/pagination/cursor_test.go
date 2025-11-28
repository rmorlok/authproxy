package pagination

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/stretchr/testify/require"
)

type FakeCursor struct {
	Value string `json:"value"`
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()
	key := config.KeyDataValue{
		Value: "0123456789abcdef0123456789abcdef",
	}

	cursor, err := MakeCursor(context.Background(), &key, &FakeCursor{
		Value: "some-value",
	})
	require.NoError(t, err)
	require.NotEmpty(t, cursor)

	parsed, err := ParseCursor[FakeCursor](context.Background(), &key, cursor)
	require.NoError(t, err)
	require.Equal(t, "some-value", parsed.Value)
}
