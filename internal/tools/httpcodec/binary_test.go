package httpcodec

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecodeBase64PreservesBytesAndPaddingLimits checks exact decoded sizes,
// ignored line breaks, and binary data that is not UTF-8.
func TestDecodeBase64PreservesBytesAndPaddingLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		encoded string
		want    []byte
		limit   int
	}{
		{"empty", "", []byte{}, 1},
		{"one byte", "Zg==", []byte("f"), 1},
		{"two bytes", "Zm8=", []byte("fo"), 2},
		{"three bytes", "Zm9v", []byte("foo"), 3},
		{"line breaks", "\rZ\ng=\r=\n", []byte("f"), 1},
		{"only line breaks", "\r\n", []byte{}, 1},
		{"binary", "AP+A", []byte{0, 255, 128}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Successful decoding respects the real size despite base64 padding.
			decoded, err := DecodeBase64(context.Background(), tc.encoded, 64, tc.limit)
			require.NoError(t, err)
			require.Equal(t, tc.want, decoded)
		})
	}
}

// TestDecodeBase64RejectsInvalidAndOversizedInput verifies malformed data and
// either size limit fail without exposing partial decoded bytes or input text.
func TestDecodeBase64RejectsInvalidAndOversizedInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		encoded string
		input   int
		output  int
		fail    string
	}{
		{"input limit", "Zm9v", 3, 3, "encoded byte limit"},
		{"newline input limit", "Zg==\n", 4, 1, "encoded byte limit"},
		{"decoded limit", "Zm9v", 4, 2, "decoded byte limit"},
		{"zero input limit", "", 0, 1, "positive"},
		{"negative input limit", "", -1, 1, "positive"},
		{"zero output limit", "", 1, 0, "positive"},
		{"negative output limit", "", 1, -1, "positive"},
		{"bad alphabet", "private-config", 64, 64, "valid standard base64"},
		{"bad padding bits", "Zh==", 64, 64, "valid standard base64"},
		{"bad two byte padding bits", "Zm9=", 64, 64, "valid standard base64"},
		{"missing padding", "Zg", 64, 64, "valid standard base64"},
		{"padding only", "====", 64, 64, "valid standard base64"},
		{"too much padding", "A===", 64, 64, "valid standard base64"},
		{"padding in middle", "Zg==AAAA", 64, 64, "valid standard base64"},
		{"URL alphabet", "____", 64, 64, "valid standard base64"},
		{"space", "Z g==", 64, 64, "valid standard base64"},
		{"tab", "Z\tg==", 64, 64, "valid standard base64"},
		{"partial data", "Zm9v!!!!", 64, 64, "valid standard base64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Invalid input must never return a successfully decoded prefix.
			decoded, err := DecodeBase64(context.Background(), tc.encoded, tc.input, tc.output)
			require.ErrorContains(t, err, tc.fail)
			require.Nil(t, decoded)
			require.NotContains(t, err.Error(), "private-config")
		})
	}
}

// TestDecodeBase64ContextErrors preserves cancellation and rejects absent
// caller contexts before any decoding can take place.
func TestDecodeBase64ContextErrors(t *testing.T) {
	// Already-canceled invocations fail even when the input is otherwise valid.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decoded, err := DecodeBase64(ctx, "Zg==", 4, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, decoded)

	// A missing context is an ordinary error rather than a panic.
	decoded, err = DecodeBase64(nil, "Zg==", 4, 1)
	require.Error(t, err)
	require.Nil(t, decoded)
}

// FuzzDecodeBase64 compares bounded decoding with the strict standard decoder,
// including malformed padding that must never overrun the exact-size buffer.
func FuzzDecodeBase64(f *testing.F) {
	for _, encoded := range []string{"", "Zg==", "Zm8=", "Zm9v", "\rZ\ng=\r=\n", "AAAA====", "A==="} {
		f.Add(encoded)
	}
	f.Fuzz(func(t *testing.T, encoded string) {
		// Every accepted source within the cap matches standard-library bytes.
		const limit = 4096
		decoded, err := DecodeBase64(context.Background(), encoded, limit, limit)
		if len(encoded) > limit {
			require.Error(t, err)
			require.Nil(t, decoded)
			return
		}
		want, wantErr := base64.StdEncoding.Strict().DecodeString(encoded)
		if wantErr != nil {
			require.Error(t, err)
			require.Nil(t, decoded)
			return
		}
		require.NoError(t, err)
		require.Equal(t, want, decoded)
	})
}
