package httperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithInternalErr_PlainError(t *testing.T) {
	inner := errors.New("db failure")
	e := InternalServerError(WithInternalErr(inner))
	require.Equal(t, inner, e.InternalErr)
}

func TestWithInternalErr_DirectHttpError(t *testing.T) {
	inner := &Error{
		Status:      http.StatusTeapot,
		ResponseMsg: "I'm a teapot",
		InternalErr: errors.New("brew error"),
	}
	e := InternalServerError(WithInternalErr(inner))
	// Should absorb fields from the direct *Error
	require.Equal(t, "I'm a teapot", e.ResponseMsg)
	require.Equal(t, errors.New("brew error").Error(), e.InternalErr.Error())
}

func TestWithInternalErr_WrappedHttpError(t *testing.T) {
	inner := &Error{
		Status:      http.StatusTeapot,
		ResponseMsg: "I'm a teapot",
		InternalErr: errors.New("brew error"),
	}
	wrapped := fmt.Errorf("outer: %w", inner)
	e := InternalServerError(WithInternalErr(wrapped))
	require.Equal(t, "I'm a teapot", e.ResponseMsg)
	require.Equal(t, wrapped, e.InternalErr) // preserves full chain
}

func TestWithResponseMsg(t *testing.T) {
	e := InternalServerError(WithResponseMsg("custom message"))
	require.Equal(t, "custom message", e.ResponseMsg)
}

func TestWithResponseMsgf(t *testing.T) {
	e := InternalServerError(WithResponseMsgf("error: %s", "details"))
	require.Equal(t, "error: details", e.ResponseMsg)
}

func TestWithPublicErr(t *testing.T) {
	inner := errors.New("visible error")
	e := BadRequest("ignored", WithPublicErr(inner))
	require.Equal(t, "visible error", e.ResponseMsg)
	require.Equal(t, inner, e.InternalErr)
}

func TestWithWrap(t *testing.T) {
	inner := errors.New("original")
	e := InternalServerError(WithWrap(inner, "context"))
	require.Equal(t, "context: original", e.InternalErr.Error())
	require.True(t, errors.Is(e.InternalErr, inner))
}

func TestWithWrapf(t *testing.T) {
	inner := errors.New("original")
	e := InternalServerError(WithWrapf(inner, "step %d", 3))
	require.Equal(t, "step 3: original", e.InternalErr.Error())
}

func TestWithWrap_HttpError(t *testing.T) {
	inner := &Error{
		Status:      http.StatusNotFound,
		ResponseMsg: "not found",
		InternalErr: errors.New("detail"),
	}
	e := InternalServerError(WithWrap(inner, "fetching"))
	require.Equal(t, "not found", e.ResponseMsg)
	require.Contains(t, e.InternalErr.Error(), "fetching")
}

func TestWithInternalErrf(t *testing.T) {
	inner := errors.New("db fail")
	e := InternalServerError(WithInternalErrf("query failed: %w", inner))
	require.Equal(t, "query failed: db fail", e.InternalErr.Error())
	require.True(t, errors.Is(e.InternalErr, inner))
}

func TestOptions_DoNotOverrideConstructorMsg(t *testing.T) {
	// WithInternalErr from a direct *Error should NOT override the constructor's message
	inner := &Error{ResponseMsg: "inner msg"}
	e := BadRequest("explicit msg", WithInternalErr(inner))
	require.Equal(t, "explicit msg", e.ResponseMsg)
}
