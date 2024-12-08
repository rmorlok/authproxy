package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"net/http"
)

type CallbackRoutes struct {
	cfg         config.C
	authService *auth.Service
}

type CallbackQueryParams struct {
	Code  string `form:"code"`
	State string `form:"state"`
}

func (r *CallbackRoutes) callback(ctx *gin.Context) {
	var req CallbackQueryParams
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type RedirectParams struct {
	Id       string `form:"id"`
	ReturnTo string `form:"return_to"`
}

func (r *CallbackRoutes) redirect(ctx *gin.Context) {
	var req RedirectParams
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (r *CallbackRoutes) Register(g *gin.Engine) {
	g.GET("/callback", r.authService.Required(), r.callback)
	g.GET("/redirect", r.authService.Required(), r.redirect)
}

func NewCallbackRoutes(cfg config.C, authService *auth.Service) *CallbackRoutes {
	return &CallbackRoutes{
		cfg:         cfg,
		authService: authService,
	}
}
