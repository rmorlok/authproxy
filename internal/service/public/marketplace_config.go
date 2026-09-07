package admin_api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const marketplaceConfigPath = "/api/v1/marketplace/config"

type marketplaceConfigResponse struct {
	ColorMode sconfig.MarketplaceColorMode `json:"colorMode"`
}

// registerMarketplaceConfigRoute exposes non-secret deployment settings used
// before the Marketplace establishes a browser session.
func registerMarketplaceConfigRoute(router gin.IRoutes, marketplace *sconfig.Marketplace) {
	router.GET(marketplaceConfigPath, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.PureJSON(http.StatusOK, marketplaceConfigResponse{
			ColorMode: marketplace.GetColorMode(),
		})
	})
}
