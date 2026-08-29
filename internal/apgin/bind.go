package apgin

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

type resourceLifecycleValidator interface {
	ValidateFor(meta.ValidationMode, *common.ValidationContext) error
}

type actionRequestValidator interface {
	ValidateRequest(meta.Kind) error
}

type actionResponseValidator interface {
	ValidateResponse(meta.Kind) error
}

// BindJSONBody strictly decodes one AuthProxy JSON request body. It preserves
// the bytes in both the request and Gin context so request logging and later
// middleware observe exactly what the client sent.
func BindJSONBody(gctx *gin.Context, obj any) error {
	data, err := io.ReadAll(gctx.Request.Body)
	if err != nil {
		return err
	}

	gctx.Request.Body = io.NopCloser(bytes.NewReader(data))
	gctx.Set(gin.BodyBytesKey, data)

	return util.DecodeJSONStrict(data, obj)
}

// BindOptionalJSONBody decodes a body when one is present.
func BindOptionalJSONBody(gctx *gin.Context, obj any) error {
	if gctx.Request == nil || gctx.Request.Body == nil || gctx.Request.Body == http.NoBody || gctx.Request.ContentLength == 0 {
		return nil
	}

	return BindJSONBody(gctx, obj)
}

// BindResourceJSON decodes and validates a resource create or update body.
// Secret placeholders emitted by redacted API responses are never accepted as
// write values.
func BindResourceJSON(gctx *gin.Context, obj resourceLifecycleValidator, mode meta.ValidationMode) error {
	if mode != meta.ValidationModeCreate && mode != meta.ValidationModeUpdate {
		return fmt.Errorf("resource request validation mode must be create or update, got %q", mode)
	}
	if err := BindJSONBody(gctx, obj); err != nil {
		return err
	}
	if err := apserde.ValidateNoRedactedPlaceholders(obj); err != nil {
		return err
	}
	return obj.ValidateFor(mode, &common.ValidationContext{Path: "$"})
}

// BindActionJSON decodes and validates a typed action request, including the
// rule that status is server-owned.
func BindActionJSON(gctx *gin.Context, obj actionRequestValidator, expectedKind meta.Kind) error {
	if err := BindJSONBody(gctx, obj); err != nil {
		return err
	}
	if err := apserde.ValidateNoRedactedPlaceholders(obj); err != nil {
		return err
	}
	return obj.ValidateRequest(expectedKind)
}

// RenderResourceJSON validates a resource as a server response before using
// the standard redacting API renderer.
func RenderResourceJSON(gctx *gin.Context, code int, obj resourceLifecycleValidator) error {
	if err := obj.ValidateFor(meta.ValidationModeResponse, &common.ValidationContext{Path: "$"}); err != nil {
		return fmt.Errorf("validate API resource response: %w", err)
	}
	APIJSON(gctx, code, obj)
	return nil
}

// RenderActionJSON validates an action response before using the standard
// redacting API renderer.
func RenderActionJSON(gctx *gin.Context, code int, obj actionResponseValidator, expectedKind meta.Kind) error {
	if err := obj.ValidateResponse(expectedKind); err != nil {
		return fmt.Errorf("validate API action response: %w", err)
	}
	APIJSON(gctx, code, obj)
	return nil
}
