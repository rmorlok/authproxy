package api_common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apctx"
)

const (
	DebugHeader = "x-authproxy-debug"
)

func PrintRoutes(g *gin.Engine) {
	for _, route := range g.Routes() {
		fmt.Printf("Method: %s, Path: %s, Handler: %s\n", route.Method, route.Path, route.Handler)
	}
}

func AddGinDebugHeader(gctx *gin.Context, debugMessage string) {
	if apctx.IsDebugMode(gctx.Request.Context()) {
		gctx.Header(DebugHeader, debugMessage)
	}
}

func AddDebugHeader(ctx context.Context, w http.ResponseWriter, debugMessage string) {
	if apctx.IsDebugMode(ctx) {
		w.Header().Set(DebugHeader, debugMessage)
	}
}

func AddGinDebugHeaderError(gctx *gin.Context, err error) {
	AddGinDebugHeader(gctx, err.Error())
}

func AddDebugHeaderError(ctx context.Context, w http.ResponseWriter, err error) {
	AddDebugHeader(ctx, w, err.Error())
}
