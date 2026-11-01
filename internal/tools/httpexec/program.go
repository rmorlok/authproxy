// Package httpexec compiles and executes an HTTP Tool after invocation admission.
// It joins schema validation, request rendering, connection I/O, and rich results.
package httpexec

import (
	"errors"
	"fmt"

	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// Limits contains operator execution policy. Compile lowers these limits with
// the definition's optional caps; a Tool cannot raise them. The component limits
// retain their own units and meanings; no defaults are supplied here.
type Limits struct {
	Render     render.Limits            // Context copying and complete request rendering.
	Validation validation.Limits        // Argument and structured-output JSON validation.
	Response   httpcodec.ResponseLimits // Response mapping, projection, and result envelope.
	HTTP       httphost.Limits          // Total invocation timeout and bounded connection I/O.
}

// Program owns one HTTP definition's compiled request, response, and schemas.
// It is immutable and may be reused concurrently for that captured revision and
// effective policy. Each Execute call creates a new session and private snapshot.
type Program struct {
	request  *httpcodec.Request  // Outbound method, URL, headers, query, and body program.
	response *httpcodec.Response // Status policy, rich passthrough, and optional projection.
	schemas  *validation.Program // Input schema and optional successful-output schema.
	limits   Limits              // Copied policy; HTTP response cap also respects the mapper cap.
}

// Compile prepares an HTTP executor without sending requests or running authored
// JavaScript. The caller owns full resource validation, authorization, revision
// selection, and operator policy. Compile validates definition.Limits and applies
// its smaller caps before compiling the components. This compiler accepts exactly
// the proxyHttp executor and retains no caller-owned definition data. Compiled
// programs must not be reused for a different revision or policy.
func Compile(definition toolschema.ToolDefinition, limits Limits) (*Program, error) {
	if definition.ProxyHTTP == nil || definition.Javascript != nil {
		return nil, errors.New("HTTP Tool requires only the proxyHttp executor")
	}

	limits, err := ResolveLimits(limits, definition.Limits)
	if err != nil {
		return nil, err
	}

	if limits.HTTP.Timeout <= 0 ||
		limits.HTTP.MaxRequests <= 0 ||
		limits.HTTP.MaxResponseBytes <= 0 ||
		limits.HTTP.MaxStreamBytes <= 0 ||
		limits.HTTP.MaxHeaderBytes <= 0 {
		return nil, errors.New("HTTP Tool host limits must all be positive")
	}

	schemas, err := validation.Compile(definition.InputSchema, definition.OutputSchema, limits.Validation)
	if err != nil {
		return nil, err
	}

	request, err := httpcodec.CompileRequest(definition.ProxyHTTP, limits.Render)
	if err != nil {
		return nil, fmt.Errorf("HTTP Tool request: %w", err)
	}

	response, err := httpcodec.CompileResponse(definition.ProxyHTTP.Response, limits.Response)
	if err != nil {
		return nil, fmt.Errorf("HTTP Tool response: %w", err)
	}

	// This executor performs one logical request. Enforce the smaller response
	// allowance during reading, before allocating a body the mapper would reject.
	limits.HTTP.MaxResponseBytes = min(limits.HTTP.MaxResponseBytes, int64(limits.Response.MaxResponseBytes))

	return &Program{
		request:  request,
		response: response,
		schemas:  schemas,
		limits:   limits,
	}, nil
}
