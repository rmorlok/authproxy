package admin_api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/routes"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// TestGetCorsConfigExposesProxyEvidence checks browser exposure without expanding access.
func TestGetCorsConfigExposesProxyEvidence(t *testing.T) {
	// No configured policy means CORS stays disabled even though proxy routes exist.
	root := &sconfig.Root{}
	require.Nil(t, GetCorsConfig(config.FromRoot(root)))

	// A configured policy keeps its permissions and existing exposed headers.
	allowCredentials := true
	root.Api.CorsVal = &sconfig.CorsConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"POST"},
		AllowedHeaders:   []string{"Authorization", "X-AuthProxy-Upstream-URL"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: &allowCredentials,
	}
	corsConfig := GetCorsConfig(config.FromRoot(root))
	expected := root.Api.CorsVal.ToGinCorsConfig(nil)
	expected.ExposeHeaders = []string{"X-Request-Id", routes.HeaderSendEvidence}
	require.Equal(t, expected, corsConfig)
	require.Equal(t, []string{"X-Request-Id"}, root.Api.CorsVal.ExposedHeaders)

	// Browser responses expose both headers, and unapproved origins remain denied.
	router := gin.New()
	router.Use(apgin.NewCorsMiddleware(*corsConfig, nil))
	router.POST("/proxy", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	request.Header.Set("Origin", "https://app.example.com")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, strings.ToLower(strings.Join(expected.ExposeHeaders, ",")), strings.ToLower(response.Header().Get("Access-Control-Expose-Headers")))
	require.Equal(t, "true", response.Header().Get("Access-Control-Allow-Credentials"))

	request.Header.Set("Origin", "https://unapproved.example.com")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}
