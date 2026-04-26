package core

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/aptmpl"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// GetDataSource fetches data from an external API through the connection's authenticated proxy,
// transforms the response using a JavaScript expression, and returns options for form dropdowns.
func (c *connection) GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error) {
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return nil, httperr.BadRequest("connection has no active setup step")
	}

	if setupStep.Phase() != cschema.SetupPhaseConfigure {
		return nil, httperr.BadRequest("data sources are only available during configure steps")
	}

	connector := c.cv.GetDefinition()
	if connector == nil || connector.SetupFlow == nil {
		return nil, httperr.BadRequest("connector has no setup flow")
	}

	step, _, err := connector.SetupFlow.GetStepBySetupStep(*setupStep)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get current step: %w", err))
	}

	ds, ok := step.DataSources[sourceId]
	if !ok {
		return nil, httperr.NotFoundf("data source %q not found in current step", sourceId)
	}

	if ds.ProxyRequest == nil {
		return nil, httperr.InternalServerErrorMsg("data source has no proxy_request defined")
	}

	// Get mustache context for template rendering
	mustacheCtx, err := c.GetMustacheContext(ctx)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get mustache context: %w", err))
	}

	// Render URL template
	renderedUrl, err := aptmpl.RenderMustache(ds.ProxyRequest.Url, mustacheCtx)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to render data source URL template: %w", err))
	}

	// Render header templates
	renderedHeaders := make(map[string]string, len(ds.ProxyRequest.Headers))
	for k, v := range ds.ProxyRequest.Headers {
		rendered, err := aptmpl.RenderMustache(v, mustacheCtx)
		if err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to render header %q template: %w", k, err))
		}
		renderedHeaders[k] = rendered
	}

	// Build and execute proxy request
	proxyReq := &iface.ProxyRequest{
		URL:     renderedUrl,
		Method:  ds.ProxyRequest.Method,
		Headers: renderedHeaders,
	}

	proxyResp, err := c.ProxyRequest(ctx, httpf.RequestTypeProxy, proxyReq)
	if err != nil {
		return nil, httperr.New(http.StatusBadGateway, "failed to fetch data source", httperr.WithInternalErrorf("data source proxy request failed: %w", err))
	}

	if proxyResp.StatusCode < 200 || proxyResp.StatusCode >= 300 {
		return nil, httperr.Newf(http.StatusBadGateway, "data source returned status %d", proxyResp.StatusCode)
	}

	// Get response data for JS transform
	var responseData any
	if proxyResp.BodyJson != nil {
		responseData = proxyResp.BodyJson
	} else {
		return nil, httperr.New(http.StatusBadGateway, "data source did not return JSON response")
	}

	// Run JavaScript transform
	options, err := apjs.TransformJSON(ds.Transform, responseData)
	if err != nil {
		return nil, httperr.InternalServerErrorMsg("failed to transform data source response", httperr.WithInternalErrorf("data source transform failed: %w", err))
	}

	return options, nil
}
