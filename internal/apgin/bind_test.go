package apgin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apserde"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

const bindTestResourceKind meta.Kind = "Widget"

type bindTestResource struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta         `json:"metadata" yaml:"metadata"`
	Spec          bindTestResourceSpec    `json:"spec" yaml:"spec"`
	Status        *bindTestResourceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type bindTestResourceSpec struct {
	Secret string `json:"secret" yaml:"secret" apiredact:"secret"`
}

type bindTestResourceStatus struct {
	Ready bool `json:"ready" yaml:"ready"`
}

func (r *bindTestResource) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	var result *multierror.Error
	if err := meta.ValidateResource(r.TypeMeta, r.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       bindTestResourceKind,
		RequireID:          mode == meta.ValidationModeResponse,
		RequireName:        true,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(r.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func TestBindJSONBodyIsStrictAndPreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"connector_id":"conn_1"}`
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = request

	var target struct {
		ConnectorID string `json:"connectorId"`
	}
	require.Error(t, BindJSONBody(ctx, &target))

	preserved, err := io.ReadAll(ctx.Request.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(preserved))
	stored, ok := ctx.Get(gin.BodyBytesKey)
	require.True(t, ok)
	require.Equal(t, []byte(body), stored)
}

func TestBindJSONBodyRejectsMissingBodyWithoutPanicking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = &http.Request{}

	var target map[string]any
	require.ErrorIs(t, BindJSONBody(ctx, &target), io.EOF)
	require.ErrorIs(t, BindJSONBody(nil, &target), io.EOF)
}

func TestBindResourceJSONAppliesWritePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", func(ctx *gin.Context) {
		var resource bindTestResource
		if err := BindResourceJSON(ctx, &resource, meta.ValidationModeCreate); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.Status(http.StatusNoContent)
	})

	t.Run("accepts resource create", func(t *testing.T) {
		body := `{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Widget",
          "metadata":{"name":"example"},
          "spec":{"secret":"client-value"}
        }`
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		require.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("rejects server-owned status with a field path", func(t *testing.T) {
		body := `{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Widget",
          "metadata":{"name":"example"},
          "spec":{"secret":"client-value"},
          "status":{"ready":true}
        }`
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), `$.status: is server-owned`)
	})

	t.Run("rejects redacted secret placeholders", func(t *testing.T) {
		body := `{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Widget",
          "metadata":{"name":"example"},
          "spec":{"secret":"******"}
        }`
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), "redacted placeholder values")
	})
}

func TestBindActionJSONRejectsResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"WidgetRefresh",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Widget","id":"wid_1"}},
      "spec":{},
      "status":{"accepted":true}
    }`))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = request

	var action apiv1alpha1.Action[struct{}, struct {
		Accepted bool `json:"accepted" yaml:"accepted"`
	}]
	err := BindActionJSON(ctx, &action, "WidgetRefresh")
	require.ErrorContains(t, err, "$.status: is server-owned")
}

func TestRenderResourceJSONValidatesAndRedacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resource := bindTestResource{
		TypeMeta: meta.NewTypeMeta(bindTestResourceKind),
		Metadata: meta.ObjectMeta{ID: "wid_1", Name: "example"},
		Spec:     bindTestResourceSpec{Secret: "client-value"},
		Status:   &bindTestResourceStatus{Ready: true},
	}
	router := gin.New()
	router.GET("/", func(ctx *gin.Context) {
		require.NoError(t, RenderResourceJSON(ctx, http.StatusOK, &resource))
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "true", response.Header().Get(apserde.RedactedHeader))
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{"id":"wid_1","name":"example"},
      "spec":{"secret":"************"},
      "status":{"ready":true}
    }`, response.Body.String())
}

func TestRenderResourceJSONRejectsInvalidServerResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	resource := bindTestResource{
		TypeMeta: meta.NewTypeMeta(bindTestResourceKind),
		Metadata: meta.ObjectMeta{Name: "example"},
	}
	err := RenderResourceJSON(ctx, http.StatusOK, &resource)
	require.ErrorContains(t, err, "$.metadata.id: is required")
}
