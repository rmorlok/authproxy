package core

import (
	"context"
	"fmt"

	"net/http"

	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/aptmpl"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// GetDataSource fetches data from an external API through the connection's authenticated proxy,
// transforms the response using a JavaScript expression, and returns options for form dropdowns.
func (c *connection) GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error) {
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("connection has no active setup step").
			BuildStatusError()
	}

	phase, _, err := cschema.ParseSetupStep(*setupStep)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg(fmt.Sprintf("invalid setup step: %s", err)).
			BuildStatusError()
	}

	if phase != "configure" {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("data sources are only available during configure steps").
			BuildStatusError()
	}

	connector := c.cv.GetDefinition()
	if connector == nil || connector.SetupFlow == nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("connector has no setup flow").
			BuildStatusError()
	}

	step, _, err := connector.SetupFlow.GetStepBySetupStep(*setupStep)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(fmt.Errorf("failed to get current step: %w", err)).
			BuildStatusError()
	}

	ds, ok := step.DataSources[sourceId]
	if !ok {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg(fmt.Sprintf("data source %q not found in current step", sourceId)).
			BuildStatusError()
	}

	if ds.ProxyRequest == nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithResponseMsg("data source has no proxy_request defined").
			BuildStatusError()
	}

	// Get mustache context for template rendering
	mustacheCtx, err := c.GetMustacheContext(ctx)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(fmt.Errorf("failed to get mustache context: %w", err)).
			BuildStatusError()
	}

	// Render URL template
	renderedUrl, err := aptmpl.RenderMustache(ds.ProxyRequest.Url, mustacheCtx)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(fmt.Errorf("failed to render data source URL template: %w", err)).
			BuildStatusError()
	}

	// Render header templates
	renderedHeaders := make(map[string]string, len(ds.ProxyRequest.Headers))
	for k, v := range ds.ProxyRequest.Headers {
		rendered, err := aptmpl.RenderMustache(v, mustacheCtx)
		if err != nil {
			return nil, api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(fmt.Errorf("failed to render header %q template: %w", k, err)).
				BuildStatusError()
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
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusBadGateway).
			WithInternalErr(fmt.Errorf("data source proxy request failed: %w", err)).
			WithResponseMsg("failed to fetch data source").
			BuildStatusError()
	}

	if proxyResp.StatusCode < 200 || proxyResp.StatusCode >= 300 {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusBadGateway).
			WithResponseMsg(fmt.Sprintf("data source returned status %d", proxyResp.StatusCode)).
			BuildStatusError()
	}

	// Get response data for JS transform
	var responseData any
	if proxyResp.BodyJson != nil {
		responseData = proxyResp.BodyJson
	} else {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusBadGateway).
			WithResponseMsg("data source did not return JSON response").
			BuildStatusError()
	}

	// Run JavaScript transform
	options, err := apjs.TransformJSON(ds.Transform, responseData)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(fmt.Errorf("data source transform failed: %w", err)).
			WithResponseMsg("failed to transform data source response").
			BuildStatusError()
	}

	return options, nil
}
