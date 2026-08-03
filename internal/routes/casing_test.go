package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRejectSnakeCaseQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	server.Use(RejectSnakeCaseQueryParams())
	server.GET("/things", func(gctx *gin.Context) { gctx.Status(http.StatusNoContent) })

	for _, test := range []struct {
		name string
		url  string
		want int
	}{
		{name: "camel case", url: "/things?labelSelector=team%3Dapi", want: http.StatusNoContent},
		{name: "legacy snake case", url: "/things?label_selector=team%3Dapi", want: http.StatusBadRequest},
		{name: "raw proxy is opaque", url: "/connections/cxn_1/_proxyRaw?third_party_key=value", want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.url, nil))
			require.Equal(t, test.want, response.Code)
		})
	}
}
