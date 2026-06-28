package apgin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/stretchr/testify/require"
)

type apiJSONSecretPayload struct {
	Public string `json:"public"`
	HTML   string `json:"html"`
	Secret string `json:"secret" apiredact:"secret"`
}

func TestAPIJSON_RedactsAndSetsHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		APIJSON(c, http.StatusOK, apiJSONSecretPayload{
			Public: "shown",
			HTML:   "<b>",
			Secret: "secret",
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "true", w.Header().Get(apserde.RedactedHeader))
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	require.JSONEq(t, `{"public":"shown","html":"<b>","secret":"******"}`, w.Body.String())
	require.Contains(t, w.Body.String(), "<b>")
}

func TestAPIJSON_ReplayLeavesHeaderUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.Request = c.Request.WithContext(apserde.WithSecretReplay(c.Request.Context(), true))
		APIJSON(c, http.StatusOK, apiJSONSecretPayload{Secret: "secret"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, w.Header().Get(apserde.RedactedHeader))
	require.JSONEq(t, `{"public":"","html":"","secret":"secret"}`, w.Body.String())
}

func TestAPIJSON_NoBodyStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		APIJSON(c, http.StatusNoContent, apiJSONSecretPayload{Secret: "secret"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, w.Body.String())
}

func TestAPIJSON_RenderError(t *testing.T) {
	err := apiJSONRender{
		ctx:  context.Background(),
		data: make(chan int),
	}.Render(httptest.NewRecorder())

	require.Error(t, err)
}
