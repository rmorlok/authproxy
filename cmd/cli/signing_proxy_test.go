package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteJsonErrorResponse preserves the signing proxy's response envelope,
// including escaping and omission of the shared type's optional stack trace.
func TestWriteJsonErrorResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		want    string
	}{
		{name: "message", message: "upstream unavailable", want: `{"error":"upstream unavailable"}`},
		{name: "empty", want: `{"error":""}`},
		{name: "escaped", message: "bad \"request\"\n<upstream>", want: `{"error":"bad \"request\"\n\u003cupstream\u003e"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeJsonErrorResponse(recorder, http.StatusBadGateway, tc.message)
			require.Equal(t, http.StatusBadGateway, recorder.Code)
			require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			require.Equal(t, tc.want, recorder.Body.String())
		})
	}
}
