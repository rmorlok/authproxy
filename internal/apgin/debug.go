package apgin

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
)

// AddDebugHeader adds a debug header to the gin response if debug mode is enabled.
func AddDebugHeader(gctx *gin.Context, debugMessage string) {
	if apctx.IsDebugMode(gctx.Request.Context()) {
		gctx.Header("x-authproxy-debug", debugMessage)
	}
}

// AddDebugHeaderError adds a debug header with the error message if debug mode is enabled.
func AddDebugHeaderError(gctx *gin.Context, err error) {
	AddDebugHeader(gctx, err.Error())
}

// PrintRoutes prints all registered routes in a Gin engine.
func PrintRoutes(g *gin.Engine) {
	for _, route := range g.Routes() {
		fmt.Printf("Method: %s, Path: %s, Handler: %s\n", route.Method, route.Path, route.Handler)
	}
}
