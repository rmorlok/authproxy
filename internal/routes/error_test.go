package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorRoutes(t *testing.T) {
	// Setup a test configuration with ErrorPages
	root := &config.Root{
		ErrorPages: config.ErrorPages{},
		Public: config.ServicePublic{
			ServiceHttp: config.ServiceHttp{
				PortVal:    &config.StringValue{&config.StringValueDirect{Value: "8080"}},
				DomainVal:  "localhost",
				IsHttpsVal: false,
			},
		},
	}
	cfg := config.FromRoot(root)

	// Create a new ErrorRoutes instance
	errorRoutes := NewErrorRoutes(cfg)

	// Create a Gin router and register the routes
	router := gin.Default()
	errorRoutes.Register(router)

	t.Run("default error", func(t *testing.T) {
		// Create a request to the error endpoint without specifying an error
		req, err := http.NewRequest(http.MethodPost, "/error", nil)
		require.NoError(t, err)

		// Create a response recorder
		w := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(w, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal Error")
		assert.Contains(t, w.Body.String(), "An internal error has occurred")
	})

	t.Run("not found error", func(t *testing.T) {
		// Create a request to the error endpoint with not_found error
		req, err := http.NewRequest(http.MethodPost, "/error?error=not_found", nil)
		require.NoError(t, err)

		// Create a response recorder
		w := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(w, req)

		// Check the response
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Page Not Found")
		assert.Contains(t, w.Body.String(), "The page you requested could not be found")
	})

	t.Run("unauthorized error", func(t *testing.T) {
		// Create a request to the error endpoint with unauthorized error
		req, err := http.NewRequest(http.MethodPost, "/error?error=unauthorized", nil)
		require.NoError(t, err)

		// Create a response recorder
		w := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(w, req)

		// Check the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Unauthorized")
		assert.Contains(t, w.Body.String(), "You are not authorized to access this page")
	})

	t.Run("internal error", func(t *testing.T) {
		// Create a request to the error endpoint with internal_error error
		req, err := http.NewRequest(http.MethodPost, "/error?error=internal_error", nil)
		require.NoError(t, err)

		// Create a response recorder
		w := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(w, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal Error")
		assert.Contains(t, w.Body.String(), "An internal error has occurred")
	})

	t.Run("unknown error", func(t *testing.T) {
		// Create a request to the error endpoint with an unknown error
		req, err := http.NewRequest(http.MethodPost, "/error?error=unknown_error", nil)
		require.NoError(t, err)

		// Create a response recorder
		w := httptest.NewRecorder()

		// Serve the request
		router.ServeHTTP(w, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Error Occurred")
		assert.Contains(t, w.Body.String(), "An unexpected error has occurred")
	})
}
