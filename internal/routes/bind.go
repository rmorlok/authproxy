package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func bindOptionalJSONBody(gctx *gin.Context, obj any) error {
	if gctx.Request == nil || gctx.Request.Body == nil || gctx.Request.Body == http.NoBody || gctx.Request.ContentLength == 0 {
		return nil
	}

	return gctx.ShouldBindBodyWithJSON(obj)
}
