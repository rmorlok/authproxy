package apgin

import (
	"log/slog"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewCorsMiddleware wraps gin-contrib/cors with logging that surfaces
// rejected origins. The underlying library aborts non-matching CORS
// requests with a silent 403 (no body, no logging), which makes
// "stuck on login" symptoms hard to diagnose — especially when the UI
// is served from an unexpected port. When that happens, this wrapper
// emits a single Warn line with the rejected origin and the configured
// allow list so the mismatch is obvious in the server log.
func NewCorsMiddleware(config cors.Config, logger *slog.Logger) gin.HandlerFunc {
	corsHandler := cors.New(config)
	allowed := append([]string(nil), config.AllowOrigins...)

	if logger != nil {
		logger.Info("CORS enabled",
			"allowed_origins", allowed,
			"allow_credentials", config.AllowCredentials,
			"allow_all_origins", config.AllowAllOrigins,
		)
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		corsHandler(c)
		if logger == nil || origin == "" {
			return
		}
		if c.IsAborted() && c.Writer.Status() == http.StatusForbidden {
			logger.Warn("CORS: rejected request from origin",
				"origin", origin,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"allowed_origins", allowed,
				"allow_all_origins", config.AllowAllOrigins,
			)
		}
	}
}
