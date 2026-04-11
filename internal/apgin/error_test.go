package apgin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

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
			`{"error":"Forbidden","stack_trace":"internal error text.*"}`,
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

func TestWriteErr_HttpError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Request = httptest.NewRequest("GET", "/", nil)

	WriteErr(gctx, nil, httperr.NotFound("gone"))

	require.Equal(t, http.StatusNotFound, rec.Code)
}
