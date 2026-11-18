package core

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/aptmpl"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// GetDataSource fetches JSON through the connection's authenticated proxy and
// transforms it into options for an eligible configure step. The caller's
// context covers definition loading, HTTP, and JavaScript. Source is admitted
// before HTTP; the separate finite transform budget starts after the response
// arrives and bounds copied inputs, execution, and option conversion.
func (c *connection) GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error) {
	if ctx == nil {
		return nil, jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return nil, httperr.BadRequest("connection has no active setup step")
	}

	connector, err := c.connector.getDefinitionWithContext(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to load data source connector definition: %w", err))
	}
	if connector == nil || connector.SetupFlow == nil {
		return nil, httperr.BadRequest("connector has no setup flow")
	}

	if !connector.SetupFlow.IsConfigureStep(*setupStep) {
		return nil, httperr.BadRequest("data sources are only available during configure steps")
	}

	flow := c.s.buildManifestSetupFlow(c)
	if _, ok, err := flow.StepById(ctx, setupStep.Id()); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	} else if !ok {
		return nil, httperr.BadRequest("current setup step is not eligible for data sources")
	}

	step, ok := connector.SetupFlow.FindStepById(setupStep.Id())
	if !ok {
		return nil, httperr.InternalServerErrorMsg("current setup step not found in connector definition")
	}

	ds, ok := step.DataSources[sourceId]
	if !ok {
		return nil, httperr.NotFoundf("data source %q not found in current step", sourceId)
	}

	if ds.ProxyRequest == nil {
		return nil, httperr.InternalServerErrorMsg("data source has no proxy_request defined")
	}

	// Admit the raw library-plus-transform source before sending an HTTP request.
	// This also loads connection input without executing the helper library.
	limits := apjs.ConnectorDataSourceLimits()
	jsctx, err := c.getBoundedJavascriptContext(ctx, ds.Transform, limits)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get javascript context: %w", err))
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
		Headers: common.HeadersValMapFromStrings(renderedHeaders),
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proxyResp, err := c.ProxyRequest(ctx, httpf.RequestTypeProxy, proxyReq)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
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

	// The transform's execution allowance starts after HTTP; the request itself
	// retains its existing proxy policy and the caller's overall deadline.
	options, err := jsctx.TransformJSONWithContext(ctx, ds.Transform, responseData, limits)
	if err != nil {
		return nil, httperr.InternalServerErrorMsg("failed to transform data source response", httperr.WithInternalErrorf("data source transform failed: %w", err))
	}

	return options, nil
}
