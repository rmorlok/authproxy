package api_common

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddGinDebugHeader(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Debuggable
		expectedValue string
		debugMessage  string
		isDebugMode   bool
	}{
		{
			name:          "DebugModeEnabled",
			cfg:           &mockDebuggable{debug: true},
			debugMessage:  "Test Message",
			expectedValue: "Test Message",
			isDebugMode:   true,
		},
		{
			name:          "DebugModeDisabled",
			cfg:           &mockDebuggable{debug: false},
			debugMessage:  "Test Message",
			expectedValue: "",
			isDebugMode:   false,
		},
		{
			name:          "NilConfig",
			cfg:           nil,
			debugMessage:  "Test Message",
			expectedValue: "",
			isDebugMode:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.ReleaseMode)
			r := gin.Default()
			r.GET("/", func(c *gin.Context) {
				AddGinDebugHeader(tt.cfg, c, tt.debugMessage)
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
		cfg           Debuggable
		debugMessage  string
		expectedValue string
	}{
		{
			name:          "DebugModeEnabled",
			cfg:           &mockDebuggable{debug: true},
			debugMessage:  "Debug Info",
			expectedValue: "Debug Info",
		},
		{
			name:          "DebugModeDisabled",
			cfg:           &mockDebuggable{debug: false},
			debugMessage:  "Debug Info",
			expectedValue: "",
		},
		{
			name:          "NilConfig",
			cfg:           nil,
			debugMessage:  "Debug Info",
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			AddDebugHeader(tt.cfg, w, tt.debugMessage)
			result := w.Header().Get(DebugHeader)

			if result != tt.expectedValue {
				t.Errorf("Expected header %s = %s, got %s", DebugHeader, tt.expectedValue, result)
			}
		})
	}
}

func TestAddGinDebugHeaderError(t *testing.T) {
	testErr := errors.New("Sample Error")
	mockCfg := &mockDebuggable{debug: true}
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		AddGinDebugHeaderError(mockCfg, c, testErr)
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
	mockCfg := &mockDebuggable{debug: true}
	w := httptest.NewRecorder()

	AddDebugHeaderError(mockCfg, w, testErr)
	result := w.Header().Get(DebugHeader)

	if result != testErr.Error() {
		t.Errorf("Expected header %s = %s, got %s", DebugHeader, testErr.Error(), result)
	}
}
