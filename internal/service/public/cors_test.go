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
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// TestGetCorsConfigExposesProxyEvidence limits added exposure to proxy-enabled services.
func TestGetCorsConfigExposesProxyEvidence(t *testing.T) {
	// Enabling proxy routes alone must not create a cross-origin policy.
	enabled := true
	root := &sconfig.Root{}
	root.Public.EnableProxyVal = &enabled
	require.Nil(t, GetCorsConfig(config.FromRoot(root)))

	for _, useMarketplaceDefaults := range []bool{false, true} {
		name := "configured policy"
		if useMarketplaceDefaults {
			name = "marketplace defaults"
		}
		t.Run(name, func(t *testing.T) {
			// Public services without proxy routes retain their original exposure.
			root := &sconfig.Root{}
			if useMarketplaceDefaults {
				root.Marketplace = &sconfig.Marketplace{BaseUrl: common.NewStringValueDirect("https://app.example.com")}
			} else {
				root.Public.CorsVal = &sconfig.CorsConfig{
					AllowedOrigins: []string{"https://app.example.com"},
					AllowedMethods: []string{"GET"},
					AllowedHeaders: []string{"Authorization"},
					ExposedHeaders: []string{"X-Request-Id"},
				}
			}
			withoutProxy := GetCorsConfig(config.FromRoot(root))
			require.NotContains(t, withoutProxy.ExposeHeaders, routes.HeaderSendEvidence)

			// Proxy exposure adds only the evidence header to the effective policy.
			root.Public.EnableProxyVal = &enabled
			withProxy := GetCorsConfig(config.FromRoot(root))
			expected := *withoutProxy
			expected.ExposeHeaders = append(append([]string(nil), withoutProxy.ExposeHeaders...), routes.HeaderSendEvidence)
			require.Equal(t, &expected, withProxy)

			// Verify the effective middleware exposes the field on a browser request.
			router := gin.New()
			router.Use(apgin.NewCorsMiddleware(*withProxy, nil))
			router.GET("/proxy", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			request.Header.Set("Origin", "https://app.example.com")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, strings.ToLower(strings.Join(expected.ExposeHeaders, ",")), strings.ToLower(response.Header().Get("Access-Control-Expose-Headers")))
		})
	}
}
