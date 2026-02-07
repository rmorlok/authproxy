package api_common

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	DebugHeader = "x-authproxy-debug"
)

func PrintRoutes(g *gin.Engine) {
	for _, route := range g.Routes() {
		fmt.Printf("Method: %s, Path: %s, Handler: %s\n", route.Method, route.Path, route.Handler)
	}
}

func AddGinDebugHeader(cfg Debuggable, gctx *gin.Context, debugMessage string) {
	if cfg != nil && cfg.IsDebugMode() {
		gctx.Header(DebugHeader, debugMessage)
	}
}

func AddDebugHeader(cfg Debuggable, w http.ResponseWriter, debugMessage string) {
	if cfg != nil && cfg.IsDebugMode() {
		w.Header().Set(DebugHeader, debugMessage)
	}
}

func AddGinDebugHeaderError(cfg Debuggable, gctx *gin.Context, err error) {
	AddGinDebugHeader(cfg, gctx, err.Error())
}

func AddDebugHeaderError(cfg Debuggable, w http.ResponseWriter, err error) {
	AddDebugHeader(cfg, w, err.Error())
}
