package httperr

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apctx"
)

const (
	DebugHeader = "x-authproxy-debug"
)

// AddDebugHeader adds a debug header to the response if debug mode is enabled.
func AddDebugHeader(ctx context.Context, w http.ResponseWriter, debugMessage string) {
	if apctx.IsDebugMode(ctx) {
		w.Header().Set(DebugHeader, debugMessage)
	}
}

// AddDebugHeaderError adds a debug header with the error message if debug mode is enabled.
func AddDebugHeaderError(ctx context.Context, w http.ResponseWriter, err error) {
	AddDebugHeader(ctx, w, err.Error())
}
