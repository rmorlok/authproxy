package routes

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/util"
)

// bindJSONBody strictly decodes one AuthProxy request body. In particular, it
// makes legacy snake_case fields a client error instead of silently ignoring
// them. Raw JSON fields within a request remain untyped by their destination
// structs and are therefore intentionally preserved.
func bindJSONBody(gctx *gin.Context, obj any) error {
	data, err := io.ReadAll(gctx.Request.Body)
	if err != nil {
		return err
	}

	// Preserve the body for handlers or middleware which may inspect it after
	// binding, matching Gin's ShouldBindBodyWithJSON behavior.
	gctx.Request.Body = io.NopCloser(bytes.NewReader(data))
	gctx.Set(gin.BodyBytesKey, data)

	return util.DecodeJSONStrict(data, obj)
}

func bindOptionalJSONBody(gctx *gin.Context, obj any) error {
	if gctx.Request == nil || gctx.Request.Body == nil || gctx.Request.Body == http.NoBody || gctx.Request.ContentLength == 0 {
		return nil
	}

	return bindJSONBody(gctx, obj)
}
