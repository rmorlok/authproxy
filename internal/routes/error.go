package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type ErrorRoutes struct {
	cfg config.C
}

func (r *ErrorRoutes) error(gctx *gin.Context) {
	errVal := sconfig.ErrorPage(gctx.DefaultQuery("error", string(sconfig.ErrorPageInternalError)))

	vals := sconfig.ErrorTemplateValues{
		Error: errVal,
	}
	r.cfg.GetRoot().ErrorPages.RenderErrorPage(gctx, vals)
}

func (r *ErrorRoutes) Register(g gin.IRouter) {
	g.POST("/error", r.error)
}

func NewErrorRoutes(cfg config.C) *ErrorRoutes {
	return &ErrorRoutes{cfg: cfg}
}
