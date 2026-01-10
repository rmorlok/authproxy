package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorPages_urlForError(t *testing.T) {
	t.Run("with configured URLs", func(t *testing.T) {
		ep := ErrorPages{
			NotFound:      "https://example.com/404",
			Unauthorized:  "https://example.com/401",
			InternalError: "https://example.com/500",
		}

		assert.Equal(t, "https://example.com/404", ep.UrlForError(ErrorPageNotFound, "https://base.example.com"))
		assert.Equal(t, "https://example.com/401", ep.UrlForError(ErrorPageUnauthorized, "https://base.example.com"))
		assert.Equal(t, "https://example.com/500", ep.UrlForError(ErrorPageInternalError, "https://base.example.com"))
		// For unknown error types, it should fall back to the default URL
		assert.Equal(t, "https://base.example.com/error?error=unknown", ep.UrlForError("unknown", "https://base.example.com"))
	})

	t.Run("without configured URLs", func(t *testing.T) {
		ep := ErrorPages{}

		// For predefined error types, it returns an empty string if not configured
		assert.Equal(t, "", ep.UrlForError(ErrorPageNotFound, "https://base.example.com"))
		assert.Equal(t, "", ep.UrlForError(ErrorPageUnauthorized, "https://base.example.com"))
		assert.Equal(t, "", ep.UrlForError(ErrorPageInternalError, "https://base.example.com"))
		// For unknown error types, it constructs a URL
		assert.Equal(t, "https://base.example.com/error?error=unknown", ep.UrlForError("unknown", "https://base.example.com"))
	})

	t.Run("with invalid base URL", func(t *testing.T) {
		ep := ErrorPages{}

		// For predefined error types, it returns an empty string if not configured
		assert.Equal(t, "", ep.UrlForError(ErrorPageNotFound, "invalid-url"))
		assert.Equal(t, "", ep.UrlForError(ErrorPageUnauthorized, "invalid-url"))
		assert.Equal(t, "", ep.UrlForError(ErrorPageInternalError, "invalid-url"))
		// For unknown error types, it constructs a URL
		// The url.Parse function treats "invalid-url" as a path, not a host
		assert.Equal(t, "error?error=unknown", ep.UrlForError("unknown", "invalid-url"))
	})

	t.Run("with nil ErrorPages", func(t *testing.T) {
		var ep *ErrorPages

		assert.Equal(t, "https://base.example.com/error?error=not_found", ep.UrlForError(ErrorPageNotFound, "https://base.example.com"))
		assert.Equal(t, "https://base.example.com/error?error=unauthorized", ep.UrlForError(ErrorPageUnauthorized, "https://base.example.com"))
		assert.Equal(t, "https://base.example.com/error?error=internal_error", ep.UrlForError(ErrorPageInternalError, "https://base.example.com"))
		assert.Equal(t, "https://base.example.com/error?error=unknown", ep.UrlForError("unknown", "https://base.example.com"))
	})
}

func TestErrorPages_RenderRenderOrRedirect(t *testing.T) {
	// Set Gin to test mode to avoid debug output
	gin.SetMode(gin.TestMode)

	t.Run("with configured URLs", func(t *testing.T) {
		ep := ErrorPages{
			NotFound:      "https://example.com/404",
			Unauthorized:  "https://example.com/401",
			InternalError: "https://example.com/500",
		}

		// Test not found error
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageNotFound})
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "https://example.com/404", w.Header().Get("Location"))

		// Test unauthorized error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageUnauthorized})
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "https://example.com/401", w.Header().Get("Location"))

		// Test internal error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageInternalError})
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "https://example.com/500", w.Header().Get("Location"))

		// Test unknown error - should render the page
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: "unknown"})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Error Occurred")
	})

	// Note: The RenderRenderOrRedirect method has a bug where it returns early
	// if the URL is not configured, without rendering the error page.
	// This test verifies the current behavior, even though it's likely a bug.
	t.Run("without configured URLs", func(t *testing.T) {
		ep := ErrorPages{}

		// For predefined error types, it returns early without rendering
		// Test not found error
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageNotFound})
		assert.Equal(t, http.StatusOK, w.Code) // No status set because it returns early
		assert.Empty(t, w.Body.String())       // No body because it returns early

		// Test unauthorized error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageUnauthorized})
		assert.Equal(t, http.StatusOK, w.Code) // No status set because it returns early
		assert.Empty(t, w.Body.String())       // No body because it returns early

		// Test internal error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: ErrorPageInternalError})
		assert.Equal(t, http.StatusOK, w.Code) // No status set because it returns early
		assert.Empty(t, w.Body.String())       // No body because it returns early

		// For unknown error types, it renders the page
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderRenderOrRedirect(c, ErrorTemplateValues{Error: "unknown"})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Error Occurred")
	})
}

func TestErrorPages_UnmarshalJSON(t *testing.T) {
	t.Run("with simple fields", func(t *testing.T) {
		jsonData := `{
			"not_found": "https://example.com/404",
			"unauthorized": "https://example.com/401",
			"internal_error": "https://example.com/500"
		}`

		var ep ErrorPages
		err := json.Unmarshal([]byte(jsonData), &ep)
		require.NoError(t, err)

		assert.Equal(t, "https://example.com/404", ep.NotFound)
		assert.Equal(t, "https://example.com/401", ep.Unauthorized)
		assert.Equal(t, "https://example.com/500", ep.InternalError)
		assert.Nil(t, ep.Template)
	})

	t.Run("with template as string", func(t *testing.T) {
		jsonData := `{
			"template": "custom template"
		}`

		var ep ErrorPages
		err := json.Unmarshal([]byte(jsonData), &ep)
		require.NoError(t, err)

		assert.NotNil(t, ep.Template)
		val, err := ep.Template.GetValue(nil)
		require.NoError(t, err)
		assert.Equal(t, "custom template", val)
	})

	t.Run("with template as object with value", func(t *testing.T) {
		jsonData := `{
			"template": {
				"value": "custom template"
			}
		}`

		var ep ErrorPages
		err := json.Unmarshal([]byte(jsonData), &ep)
		require.NoError(t, err)

		assert.NotNil(t, ep.Template)
		val, err := ep.Template.GetValue(nil)
		require.NoError(t, err)
		assert.Equal(t, "custom template", val)
	})

	t.Run("with template as object with base64", func(t *testing.T) {
		jsonData := `{
			"template": {
				"base64": "Y3VzdG9tIHRlbXBsYXRl"
			}
		}`

		var ep ErrorPages
		err := json.Unmarshal([]byte(jsonData), &ep)
		require.NoError(t, err)

		assert.NotNil(t, ep.Template)
		val, err := ep.Template.GetValue(nil)
		require.NoError(t, err)
		assert.Equal(t, "custom template", val)
	})
}

func TestErrorPages_RenderErrorPage(t *testing.T) {
	// Set Gin to test mode to avoid debug output
	gin.SetMode(gin.TestMode)

	t.Run("with default values", func(t *testing.T) {
		ep := ErrorPages{}

		// Test not found error
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{Error: ErrorPageNotFound})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Page Not Found")
		assert.Contains(t, w.Body.String(), "The page you requested could not be found")

		// Test unauthorized error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{Error: ErrorPageUnauthorized})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Unauthorized")
		assert.Contains(t, w.Body.String(), "You are not authorized to access this page")

		// Test internal error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{Error: ErrorPageInternalError})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal Error")
		assert.Contains(t, w.Body.String(), "An internal error has occurred")

		// Test unknown error
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{Error: "unknown"})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Error Occurred")
		assert.Contains(t, w.Body.String(), "An unexpected error has occurred")
	})

	t.Run("with custom values", func(t *testing.T) {
		ep := ErrorPages{}

		// Test with custom title and description
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{
			Error:       ErrorPageNotFound,
			Title:       "Custom Not Found",
			Description: "Custom not found description",
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Custom Not Found")
		assert.Contains(t, w.Body.String(), "Custom not found description")
	})

	t.Run("with custom template", func(t *testing.T) {
		customTemplate := `
<!DOCTYPE html>
<html>
<head>
    <title>Custom: {{.Title}}</title>
</head>
<body>
    <h1>Custom: {{.Title}}</h1>
    <p>Custom: {{.Description}}</p>
</body>
</html>
`
		ep := ErrorPages{
			Template: &StringValue{&StringValueDirect{Value: customTemplate}},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/", nil)
		ep.RenderErrorPage(c, ErrorTemplateValues{Error: ErrorPageNotFound})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Custom: Page Not Found")
		assert.Contains(t, w.Body.String(), "Custom: The page you requested could not be found")
	})
}
