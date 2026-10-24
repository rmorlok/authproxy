package httpcodec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// Request is an immutable outbound HTTP request program. The URL and header
// values share one envelope program; query and body use their existing codecs.
// Method and header names are literal. Compilation captures all source data,
// so independent invocations may reuse the program concurrently.
type Request struct {
	// method is the literal HTTP method copied from the definition. For example,
	// "PATCH" becomes http.Request.Method unchanged; it is never templated.
	method string

	// names contains the authored header names in lexical order. Each names[i]
	// identifies the header whose value is at envelope position i+1. For example,
	// ["X-Tenant"] pairs with the second envelope value to produce X-Tenant: acme.
	// Names stay outside the template so a header named "$value" remains literal.
	names []string

	// envelope is the compiled program for a JSON string array: the URL first,
	// then header values in names order. With names ["X-Tenant"], a source array
	// ["https://{{cfg.host}}/items", "{{cfg.tenant}}"] might render to
	// ["https://api.example.com/items", "acme"]. Render uses the first string as
	// the destination and the remaining strings as header values. One program
	// makes all these strings share the same rendering budget.
	envelope *render.ValueTemplate

	// query is the compiled program for structured query fields, separate from
	// any query already in the URL. For example, {"tag":{"$value":"params.tags"}} with
	// params.tags = ["lead", "new"] renders to "tag=lead&tag=new", which Render
	// appends after checking for keys also defined in the URL. An empty field
	// map still has a program and produces an empty query string.
	query *Parameters

	// body is the compiled program for the selected body mode and its content type.
	// For example, bodyJson: {"active":{"$value":"params.active"}} with
	// params.active = false
	// produces bytes {"active":false} and Content-Type application/json. A
	// successfully compiled request always has a body program; an omitted body
	// uses bodyAbsent and renders with Present=false.
	body *Body

	// limits holds the positive byte, depth, and step caps captured at compile
	// time. Each program applies them independently; MaxBytes also caps the
	// combined request source and final prepared request data.
	limits render.Limits
}

// CompileRequest compiles the outbound fields of an HTTP Tool without accessing
// invocation data, credentials, or the network. Response mapping is deliberately
// left to the execution layer. Malformed literal URLs, header syntax, and known
// body/content-type conflicts fail during compilation. Final values and dynamic
// body/header compatibility are checked after rendering, before a request exists.
//
// Positive limits apply separately to the envelope, query, and body programs,
// giving at most three MaxSteps allowances. MaxBytes also bounds combined raw
// source and final prepared data. It is not an exact HTTP wire-size or memory cap.
func CompileRequest(
	source *tools.ProxyHTTP,
	limits render.Limits,
) (*Request, error) {
	if source == nil {
		return nil, errors.New("HTTP request definition is required")
	}

	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxSteps <= 0 {
		return nil, errors.New("request rendering limits must all be positive")
	}

	if err := checkRequestSource(source, limits); err != nil {
		return nil, err
	}

	if !tools.IsHTTPToken(source.Method) {
		return nil, errors.New("request method must be an HTTP token")
	}

	if !utf8.ValidString(source.URL) || strings.TrimSpace(source.URL) == "" {
		return nil, errors.New("request URL template must be nonempty valid UTF-8")
	}

	if !strings.Contains(source.URL, "{{") {
		if _, err := prepareRequestURL(source.URL, ""); err != nil {
			return nil, err
		}
	}

	names, err := requestHeaderNames(source.Headers)

	if err != nil {
		return nil, err
	}

	// Keep names outside the structured program: a literal "$value" header
	// must never be interpreted as a directive. Check UTF-8 before Marshal,
	// which would otherwise silently replace invalid authored string bytes.
	values := make([]string, 1, len(names)+1)
	values[0] = source.URL
	staticHeaders := make(map[string]string, len(names))

	for _, name := range names {
		value := source.Headers[name]
		if !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("request header templates must be valid UTF-8 without newlines")
		}

		if !strings.Contains(value, "{{") {
			staticHeaders[name] = value
		}

		values = append(values, value)
	}

	literalHeaders, _, err := prepareRequestHeaders(staticHeaders, RenderedBody{}, bodyAbsent)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, errors.New("cannot compile request envelope")
	}

	envelope, err := render.CompileValue(encoded, limits)
	if err != nil {
		return nil, err
	}

	query, err := CompileParameters(source.Query, limits)
	if err != nil {
		return nil, err
	}

	body, err := CompileBody(source, limits)
	if err != nil {
		return nil, err
	}

	if _, present := literalHeaders["Content-Type"]; present {
		generatedType := body.contentType
		if body.mode == bodyMultipart {
			// Compatibility checks need a generated boundary, but no payload is
			// rendered here. This placeholder is discarded; the encoder creates
			// the actual boundary independently for each invocation.
			generatedType = "multipart/form-data; boundary=compile-validation"
		}
		if _, err := requestContentType(
			literalHeaders.Get("Content-Type"),
			true, // present
			generatedType,
			body.mode,
		); err != nil {
			return nil, err
		}
	}
	return &Request{method: source.Method, names: names, envelope: envelope, query: query, body: body, limits: limits}, nil
}

// checkRequestSource bounds aggregate authored data before validators or JSON
// marshaling allocate their own representations. Body source accounting reuses
// the body compiler's check with the bytes left after the envelope and query.
func checkRequestSource(source *tools.ProxyHTTP, limits render.Limits) error {
	remaining := limits.MaxBytes
	add := func(size int) bool {
		if size > remaining {
			return false
		}
		remaining -= size
		return true
	}
	if !add(len(source.Method)) || !add(len(source.URL)) {
		return errors.New("request source byte limit exceeded")
	}
	if len(source.Headers) > limits.MaxSteps || len(source.Query) > limits.MaxSteps {
		return errors.New("request field count exceeds render step limit")
	}
	for name, value := range source.Headers {
		if !add(len(name)) || !add(len(value)) {
			return errors.New("request source byte limit exceeded")
		}
	}
	for name, value := range source.Query {
		if !add(len(name)) || !add(len(value)) {
			return errors.New("request source byte limit exceeded")
		}
	}
	limits.MaxBytes = remaining
	return checkBodySource(source, limits)
}

// Render prepares an independent HTTP request and validates every rendered
// component before returning it. It performs no I/O or credential application.
// The caller owns Body and must close it or transfer ownership to the proxy.
// Present bodies have GetBody readers for the proxy's existing recovery policy;
// replayability does not authorize new retries. Errors return no partial request
// and never include rendered URL/header/configuration values.
func (r *Request) Render(ctx context.Context, data *render.Context) (*http.Request, error) {
	if r == nil || r.envelope == nil || ctx == nil || data == nil {
		return nil, errors.New("request program, context, and invocation data are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	envelope, err := r.envelope.Render(ctx, data)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(envelope, &values); err != nil || len(values) != len(r.names)+1 {
		return nil, errors.New("rendered request envelope must contain string fields")
	}
	query, err := r.query.Render(ctx, data)
	if err != nil {
		return nil, err
	}
	u, err := prepareRequestURL(values[0], query)
	if err != nil {
		return nil, err
	}
	body, err := r.body.Render(ctx, data)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]string, len(r.names))
	for i, name := range r.names {
		fields[name] = values[i+1]
	}
	headers, hostOverride, err := prepareRequestHeaders(fields, body, r.body.mode)
	if err != nil {
		return nil, err
	}
	urlText := u.String()
	if err := checkPreparedRequestSize(r.method, urlText, hostOverride, headers, body.Bytes, r.limits.MaxBytes); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var reader io.Reader
	if body.Present {
		reader = bytes.NewReader(body.Bytes)
	}
	request, err := http.NewRequestWithContext(ctx, r.method, urlText, reader)
	if err != nil {
		return nil, errors.New("cannot prepare HTTP request")
	}
	request.Header = headers
	if hostOverride != "" {
		request.Host = hostOverride
	}
	return request, nil
}

// checkPreparedRequestSize counts method, serialized URL, explicit Host override,
// header names/values, and encoded body. The default authority is already counted
// in the URL. Credentials, transport framing, and automatically added HTTP headers
// belong to later layers and are not counted.
func checkPreparedRequestSize(method, url, hostOverride string, headers http.Header, body []byte, maxBytes int) error {
	remaining := maxBytes
	add := func(size int) bool {
		if size > remaining {
			return false
		}
		remaining -= size
		return true
	}
	if !add(len(method)) || !add(len(url)) || !add(len(hostOverride)) || !add(len(body)) {
		return errors.New("prepared request byte limit exceeded")
	}
	for name, values := range headers {
		if !add(len(name)) {
			return errors.New("prepared request byte limit exceeded")
		}
		for _, value := range values {
			if !add(len(value)) {
				return errors.New("prepared request byte limit exceeded")
			}
		}
	}
	return nil
}
