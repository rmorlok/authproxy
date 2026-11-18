package apgin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/stretchr/testify/require"
)

func TestWriteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name              string
		err               *httperr.Error
		debug             bool
		expectedStatus    int
		expectedBodyRegex string
	}{
		{
			"normalMode",
			httperr.Forbidden("Forbidden"),
			false,
			http.StatusForbidden,
			`{"error":"Forbidden"}`,
		},
		{
			"debug",
			&httperr.Error{Status: http.StatusForbidden, ResponseMsg: "Forbidden", InternalErr: errors.New("internal error text")},
			true,
			http.StatusForbidden,
			`{"error":"Forbidden","stackTrace":"internal error text.*"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := apctx.WithDebugMode(context.Background(), tt.debug)
			rec := httptest.NewRecorder()
			gctx, _ := gin.CreateTestContext(rec)
			gctx.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)

			WriteError(gctx, nil, tt.err)

			require.Equal(t, tt.expectedStatus, rec.Code)

			trimmedBody := strings.TrimSpace(rec.Body.String())
			matched, err := regexp.MatchString(tt.expectedBodyRegex, trimmedBody)
			require.NoError(t, err)
			require.True(t, matched, "expected body to match regex %q, but got %q", tt.expectedBodyRegex, trimmedBody)
		})
	}
}

func TestWriteErr_PlainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Request = httptest.NewRequest("GET", "/", nil)

	WriteErr(gctx, nil, errors.New("something broke"))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWriteErr_ClientClosedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gctx.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	WriteErr(gctx, nil, errors.Join(errors.New("redis write failed"), context.Canceled))

	require.Equal(t, httperr.StatusClientClosedRequest, rec.Code)
	require.Equal(t, `{"error":"Client Closed Request"}`, strings.TrimSpace(rec.Body.String()))
}

func TestWriteErr_HttpError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Request = httptest.NewRequest("GET", "/", nil)

	WriteErr(gctx, nil, httperr.NotFound("gone"))

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestWriteError_RequestValidation verifies that the shared validation error
// classifier integrates with request cancellation, response messages, and Gin's
// retained error cause without treating a deadline as a client disconnect.
func TestWriteError_RequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Time{})
	defer stop()
	invalid := errors.New("spec.name is required")
	for _, test := range []struct {
		name    string
		ctx     context.Context
		cause   error
		status  int
		message string
	}{
		{name: "client canceled", ctx: canceled, cause: context.Canceled, status: httperr.StatusClientClosedRequest, message: "Client Closed Request"},
		{name: "deadline expired", ctx: expired, cause: context.DeadlineExceeded, status: http.StatusInternalServerError, message: "Internal Server Error"},
		{name: "internal cancellation", ctx: context.Background(), cause: context.Canceled, status: http.StatusInternalServerError, message: "Internal Server Error"},
		{name: "invalid input during cancellation", ctx: canceled, cause: invalid, status: http.StatusBadRequest, message: invalid.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Supply the actual request context so only client cancellation maps to 499.
			rec := httptest.NewRecorder()
			gctx, _ := gin.CreateTestContext(rec)
			gctx.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(test.ctx)

			WriteError(gctx, nil, httperr.RequestValidationErr(test.cause, httperr.WithPublicErr(test.cause)))

			// Operational failures use server messages; real input errors stay public,
			// and all errors remain available to logging and other Gin middleware.
			require.Equal(t, test.status, rec.Code)
			require.JSONEq(t, `{"error":"`+test.message+`"}`, rec.Body.String())
			require.Len(t, gctx.Errors, 1)
			require.ErrorIs(t, gctx.Errors[0].Err, test.cause)
		})
	}
}
