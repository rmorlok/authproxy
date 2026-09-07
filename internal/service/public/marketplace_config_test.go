package admin_api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestMarketplaceConfigRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns the default system mode", func(t *testing.T) {
		router := gin.New()
		registerMarketplaceConfigRoute(router, nil)

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, marketplaceConfigPath, nil)
		router.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		require.JSONEq(t, `{"colorMode":"system"}`, response.Body.String())
	})

	t.Run("returns the configured mode", func(t *testing.T) {
		mode := sconfig.MarketplaceColorModeDark
		router := gin.New()
		registerMarketplaceConfigRoute(router, &sconfig.Marketplace{ColorMode: &mode})

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, marketplaceConfigPath, nil)
		router.ServeHTTP(response, request)

		require.Equal(t, http.StatusOK, response.Code)
		require.JSONEq(t, `{"colorMode":"dark"}`, response.Body.String())
	})
}
