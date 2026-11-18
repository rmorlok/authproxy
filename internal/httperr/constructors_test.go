package httperr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBadRequest(t *testing.T) {
	e := BadRequest("invalid input")
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "invalid input", e.ResponseMsg)
	require.Nil(t, e.InternalErr)
}

func TestBadRequestf(t *testing.T) {
	e := BadRequestf("field %s is invalid", "name")
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "field name is invalid", e.ResponseMsg)
}

func TestBadRequestErr(t *testing.T) {
	inner := errors.New("parse error")
	e := BadRequestErr(inner)
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "", e.ResponseMsg)
	require.Equal(t, inner, e.InternalErr)
}

// TestRequestValidationErr verifies that ordinary validation errors retain each
// caller's chosen disclosure policy and preserve their cause for logging.
func TestRequestValidationErr(t *testing.T) {
	inner := errors.New("spec.name is required")
	for _, test := range []struct {
		name    string
		opts    []Option
		message string
	}{
		{name: "private by default", message: "Bad Request"},
		{name: "public validation details", opts: []Option{WithPublicErr(inner)}, message: inner.Error()},
		{name: "fixed public message", opts: []Option{WithResponseMsg("invalid request body")}, message: "invalid request body"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Each route chooses how much of an ordinary validation failure to expose.
			err := RequestValidationErr(inner, test.opts...)

			// Classification does not discard the original error or change disclosure.
			require.Equal(t, http.StatusBadRequest, err.Status)
			require.Equal(t, test.message, err.ResponseMsgOrDefault())
			require.ErrorIs(t, err, inner)
		})
	}
}

// TestRequestValidationErr_ContextFailures verifies that validation wrappers and
// public-message options cannot turn cancellation into an authored input error.
func TestRequestValidationErr_ContextFailures(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		for name, inner := range map[string]error{
			"direct":     cause,
			"wrapped":    fmt.Errorf("private validation detail: %w", cause),
			"aggregated": errors.Join(errors.New("private validation detail"), cause),
			"classified": BadRequestErr(cause, WithResponseMsg("private validation detail")),
		} {
			t.Run(cause.Error()+"/"+name, func(t *testing.T) {
				// A caller may offer public validation details or a fixed input-error
				// message; neither describes an operational context failure.
				for _, opts := range [][]Option{nil, {WithPublicErr(inner)}, {WithResponseMsg("invalid request body")}} {
					err := RequestValidationErr(inner, opts...)

					// Keep the full cause chain, but let the normal server error policy
					// choose the response instead of leaking validation details.
					require.Equal(t, http.StatusInternalServerError, err.Status)
					require.Equal(t, "Internal Server Error", err.ResponseMsgOrDefault())
					require.Equal(t, inner, err.InternalErr)
					require.ErrorIs(t, err, cause)
				}
			})
		}
	}
}

func TestBadRequest_WithOptions(t *testing.T) {
	inner := errors.New("parse error")
	e := BadRequest("bad input", WithInternalErr(inner))
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "bad input", e.ResponseMsg)
	require.Equal(t, inner, e.InternalErr)
}

func TestUnauthorized(t *testing.T) {
	e := Unauthorized()
	require.Equal(t, http.StatusUnauthorized, e.Status)
	require.Equal(t, "", e.ResponseMsg)
	require.Equal(t, "Unauthorized", e.ResponseMsgOrDefault())
}

func TestUnauthorizedMsg(t *testing.T) {
	e := UnauthorizedMsg("token expired")
	require.Equal(t, http.StatusUnauthorized, e.Status)
	require.Equal(t, "token expired", e.ResponseMsg)
}

func TestForbidden(t *testing.T) {
	e := Forbidden("access denied")
	require.Equal(t, http.StatusForbidden, e.Status)
	require.Equal(t, "access denied", e.ResponseMsg)
}

func TestForbiddenf(t *testing.T) {
	e := Forbiddenf("no access to %s", "resource")
	require.Equal(t, http.StatusForbidden, e.Status)
	require.Equal(t, "no access to resource", e.ResponseMsg)
}

func TestNotFound(t *testing.T) {
	e := NotFound("actor not found")
	require.Equal(t, http.StatusNotFound, e.Status)
	require.Equal(t, "actor not found", e.ResponseMsg)
}

func TestNotFoundf(t *testing.T) {
	e := NotFoundf("actor %s not found", "abc")
	require.Equal(t, http.StatusNotFound, e.Status)
	require.Equal(t, "actor abc not found", e.ResponseMsg)
}

func TestConflict(t *testing.T) {
	e := Conflict("already exists")
	require.Equal(t, http.StatusConflict, e.Status)
	require.Equal(t, "already exists", e.ResponseMsg)
}

func TestConflictf(t *testing.T) {
	e := Conflictf("actor %s already exists", "abc")
	require.Equal(t, http.StatusConflict, e.Status)
	require.Equal(t, "actor abc already exists", e.ResponseMsg)
}

func TestInternalServerError(t *testing.T) {
	inner := errors.New("db error")
	e := InternalServerError(WithInternalErr(inner))
	require.Equal(t, http.StatusInternalServerError, e.Status)
	require.Equal(t, "", e.ResponseMsg)
	require.Equal(t, inner, e.InternalErr)
}

func TestInternalServerErrorMsg(t *testing.T) {
	e := InternalServerErrorMsg("something went wrong")
	require.Equal(t, http.StatusInternalServerError, e.Status)
	require.Equal(t, "something went wrong", e.ResponseMsg)
}

func TestNew(t *testing.T) {
	e := New(http.StatusBadGateway, "upstream failed")
	require.Equal(t, http.StatusBadGateway, e.Status)
	require.Equal(t, "upstream failed", e.ResponseMsg)
}

func TestNewf(t *testing.T) {
	e := Newf(http.StatusBadGateway, "status %d from upstream", 503)
	require.Equal(t, http.StatusBadGateway, e.Status)
	require.Equal(t, "status 503 from upstream", e.ResponseMsg)
}

func TestNewWithErr(t *testing.T) {
	inner := errors.New("connection refused")
	e := NewWithErr(http.StatusBadGateway, inner)
	require.Equal(t, http.StatusBadGateway, e.Status)
	require.Equal(t, inner, e.InternalErr)
}

func TestFromErrorf(t *testing.T) {
	inner := &Error{Status: http.StatusForbidden, ResponseMsg: "forbidden"}
	e := FromErrorf("wrapped: %w", inner)
	require.Equal(t, http.StatusForbidden, e.Status)
	require.Equal(t, "forbidden", e.ResponseMsg)
	require.Equal(t, "wrapped: forbidden", e.InternalErr.Error())
}

func TestFromErrorf_PlainError(t *testing.T) {
	inner := errors.New("db error")
	e := FromErrorf("query failed: %w", inner)
	require.Equal(t, http.StatusInternalServerError, e.Status)
	require.Equal(t, "query failed: db error", e.InternalErr.Error())
}

func TestFromError_PlainError(t *testing.T) {
	inner := errors.New("something broke")
	e := FromError(inner)
	require.Equal(t, http.StatusInternalServerError, e.Status)
	require.Equal(t, inner, e.InternalErr)
}

func TestFromError_HttpError(t *testing.T) {
	orig := &Error{Status: http.StatusBadRequest, ResponseMsg: "bad", InternalErr: errors.New("detail")}
	e := FromError(orig)
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "bad", e.ResponseMsg)
	require.Equal(t, errors.New("detail").Error(), e.InternalErr.Error())
}

func TestFromError_WrappedHttpError(t *testing.T) {
	orig := &Error{Status: http.StatusBadRequest, ResponseMsg: "bad"}
	wrapped := fmt.Errorf("outer: %w", orig)
	e := FromError(wrapped)
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, "bad", e.ResponseMsg)
	require.Equal(t, "outer: bad", e.InternalErr.Error())
}

func TestFromError_NilError(t *testing.T) {
	e := FromError(nil)
	require.Equal(t, http.StatusInternalServerError, e.Status)
	require.Nil(t, e.InternalErr)
}

func TestFromError_DoesNotMutateOriginal(t *testing.T) {
	orig := &Error{Status: http.StatusBadRequest, ResponseMsg: "original"}
	e := FromError(orig, WithResponseMsg("changed"))
	require.Equal(t, "changed", e.ResponseMsg)
	require.Equal(t, "original", orig.ResponseMsg)
}

func TestErrorImplementsErrorInterface(t *testing.T) {
	var err error = BadRequest("test")
	require.NotNil(t, err)
	require.Equal(t, "test", err.Error())
}
