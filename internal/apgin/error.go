package apgin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// WriteError writes an *httperr.Error to the gin context with logging,
// debug headers, and error attachment.
func WriteError(gctx *gin.Context, logger *slog.Logger, err *httperr.Error) {
	err.LogError(logger)

	ctx := gctx.Request.Context()

	if err.InternalErr != nil {
		AddDebugHeaderError(gctx, err.InternalErr)
		_ = gctx.Error(err.InternalErr)
	}

	errorResponse := err.ToErrorResponse(ctx)
	gctx.Header("Content-Type", "application/json")
	gctx.PureJSON(err.Status, errorResponse)
}

// WriteErr writes any error to the gin context. If err is an *httperr.Error,
// it is written directly; otherwise it is wrapped as a 500.
func WriteErr(gctx *gin.Context, logger *slog.Logger, err error) {
	WriteError(gctx, logger, httperr.FromError(err))
}
