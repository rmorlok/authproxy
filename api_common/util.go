package api_common

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/config"
)

func PrintRoutes(g *gin.Engine) {
	for _, route := range g.Routes() {
		fmt.Printf("Method: %s, Path: %s, Handler: %s\n", route.Method, route.Path, route.Handler)
	}
}

func AddDebugHeader(cfg config.C, gctx *gin.Context, debugMessage string) {
	if cfg.IsDebugMode() {
		gctx.Header("x-authproxy-debug", debugMessage)
	}
}

func AddDebugHeaderError(cfg config.C, gctx *gin.Context, err error) {
	AddDebugHeader(cfg, gctx, err.Error())
}
