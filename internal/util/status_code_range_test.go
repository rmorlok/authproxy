package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHTTPStatusCodeRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantStart int
		wantEnd   int
	}{
		{name: "single code", raw: "404", wantStart: 404, wantEnd: 404},
		{name: "inclusive range", raw: "200-299", wantStart: 200, wantEnd: 299},
		{name: "status family", raw: "5xx", wantStart: 500, wantEnd: 599},
		{name: "trimmed uppercase family", raw: " 2XX ", wantStart: 200, wantEnd: 299},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end, err := ParseHTTPStatusCodeRange(tt.raw)
			require.NoError(t, err)
			require.Equal(t, tt.wantStart, start)
			require.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestParseHTTPStatusCodeRangeRejectsInvalidFamily(t *testing.T) {
	t.Parallel()

	_, _, err := ParseHTTPStatusCodeRange("$status")
	require.Error(t, err)

	_, _, err = ParseHTTPStatusCodeRange("9xx")
	require.Error(t, err)
}
