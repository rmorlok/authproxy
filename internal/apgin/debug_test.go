package apgin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/require"
)

func TestAddDebugHeader(t *testing.T) {
	tests := []struct {
		name          string
		debug         bool
		message       string
		expectedValue string
	}{
		{"enabled", true, "Test Message", "Test Message"},
		{"disabled", false, "Test Message", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := apctx.WithDebugMode(context.Background(), tt.debug)
			gin.SetMode(gin.ReleaseMode)
			r := gin.Default()
			r.GET("/", func(c *gin.Context) {
				c.Request = c.Request.WithContext(ctx)
				AddDebugHeader(c, tt.message)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			r.ServeHTTP(w, req)
			result := w.Header().Get("x-authproxy-debug")

			require.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestAddDebugHeaderError(t *testing.T) {
	testErr := errors.New("Sample Error")
	ctx := apctx.WithDebugMode(context.Background(), true)
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		AddDebugHeaderError(c, testErr)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	result := w.Header().Get("x-authproxy-debug")

	require.Equal(t, testErr.Error(), result)
}
