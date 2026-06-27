package apgin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apserde"
)

var apiJSONContentType = []string{"application/json; charset=utf-8"}

// APIJSON serializes obj as an API JSON response, applying apiredact tags unless
// the request context is authorized for secret replay.
func APIJSON(gctx *gin.Context, code int, obj any) {
	gctx.Render(code, apiJSONRender{
		ctx:  gctx.Request.Context(),
		data: obj,
	})
}

type apiJSONRender struct {
	ctx  context.Context
	data any
}

func (r apiJSONRender) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	data, report, err := apserde.MarshalJSONForAPI(r.ctx, r.data)
	if err != nil {
		return err
	}
	if report.Redacted {
		w.Header().Set(apserde.RedactedHeader, "true")
	}
	_, err = w.Write(data)
	return err
}

func (r apiJSONRender) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if len(header["Content-Type"]) == 0 {
		header["Content-Type"] = apiJSONContentType
	}
}
