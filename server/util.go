package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
)

func PrintRoutes(g *gin.Engine) {
	for _, route := range g.Routes() {
		fmt.Printf("Method: %s, Path: %s, Handler: %s\n", route.Method, route.Path, route.Handler)
	}
}
