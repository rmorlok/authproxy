package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBindJSONBodyRejectsLegacySnakeCaseKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"connector_id":"conn_1"}`))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = request

	var body struct {
		ConnectorID string `json:"connectorId"`
	}
	require.Error(t, bindJSONBody(ctx, &body))
}
