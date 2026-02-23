package api_common

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
)

func TestAddGinDebugHeader(t *testing.T) {
	tests := []struct {
		name          string
		debug         bool
		expectedValue string
		debugMessage  string
	}{
		{
			name:          "DebugModeEnabled",
			debug:         true,
			debugMessage:  "Test Message",
			expectedValue: "Test Message",
		},
		{
			name:          "DebugModeDisabled",
			debug:         false,
			debugMessage:  "Test Message",
			expectedValue: "",
		},
		{
			name:          "DefaultContext",
			debug:         false,
			debugMessage:  "Test Message",
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := apctx.WithDebugMode(context.Background(), tt.debug)
			gin.SetMode(gin.ReleaseMode)
			r := gin.Default()
			r.GET("/", func(c *gin.Context) {
				c.Request = c.Request.WithContext(ctx)
				AddGinDebugHeader(c, tt.debugMessage)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			r.ServeHTTP(w, req)
			result := w.Header().Get(DebugHeader)

			if result != tt.expectedValue {
				t.Errorf("Expected header %s = %s, got %s", DebugHeader, tt.expectedValue, result)
			}
		})
	}
}

func TestAddDebugHeader(t *testing.T) {
	tests := []struct {
		name          string
		debug         bool
		debugMessage  string
		expectedValue string
	}{
		{
			name:          "DebugModeEnabled",
			debug:         true,
			debugMessage:  "Debug Info",
			expectedValue: "Debug Info",
		},
		{
			name:          "DebugModeDisabled",
			debug:         false,
			debugMessage:  "Debug Info",
			expectedValue: "",
		},
		{
			name:          "DefaultContext",
			debug:         false,
			debugMessage:  "Debug Info",
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := apctx.WithDebugMode(context.Background(), tt.debug)
			w := httptest.NewRecorder()
			AddDebugHeader(ctx, w, tt.debugMessage)
			result := w.Header().Get(DebugHeader)

			if result != tt.expectedValue {
				t.Errorf("Expected header %s = %s, got %s", DebugHeader, tt.expectedValue, result)
			}
		})
	}
}

func TestAddGinDebugHeaderError(t *testing.T) {
	testErr := errors.New("Sample Error")
	ctx := apctx.WithDebugMode(context.Background(), true)
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.Request = c.Request.WithContext(ctx)
		AddGinDebugHeaderError(c, testErr)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	result := w.Header().Get(DebugHeader)

	if result != testErr.Error() {
		t.Errorf("Expected header %s = %s, got %s", DebugHeader, testErr.Error(), result)
	}
}

func TestAddDebugHeaderError(t *testing.T) {
	testErr := errors.New("Another Error")
	ctx := apctx.WithDebugMode(context.Background(), true)
	w := httptest.NewRecorder()

	AddDebugHeaderError(ctx, w, testErr)
	result := w.Header().Get(DebugHeader)

	if result != testErr.Error() {
		t.Errorf("Expected header %s = %s, got %s", DebugHeader, testErr.Error(), result)
	}
}
