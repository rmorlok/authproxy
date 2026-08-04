package routes

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// RejectSnakeCaseQueryParams rejects legacy AuthProxy query parameter names.
// Raw proxy requests are deliberately excluded because their query string is
// an opaque third-party payload that AuthProxy forwards unchanged.
func RejectSnakeCaseQueryParams() gin.HandlerFunc {
	return func(gctx *gin.Context) {
		if strings.HasPrefix(gctx.Request.URL.Path, "/oauth2/callback") ||
			strings.HasSuffix(gctx.Request.URL.Path, "/_proxy") ||
			strings.HasSuffix(gctx.Request.URL.Path, "/_proxyRaw") {
			gctx.Next()
			return
		}

		for key := range gctx.Request.URL.Query() {
			if strings.Contains(key, "_") {
				gctx.Abort()
				apgin.WriteError(gctx, nil, httperr.BadRequestf("query parameter %q must use lowerCamelCase", key))
				return
			}
		}

		gctx.Next()
	}
}
