package tools

import (
	"strings"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// requireValidationPath checks the typed field path, including a caller's
// prefix, rather than depending on the formatting of the aggregate message.
func requireValidationPath(t *testing.T, err error, path string) {
	t.Helper()
	require.Error(t, err)
	var fieldError *common.ValidationError
	require.ErrorAs(t, err, &fieldError)
	require.Equal(t, path, fieldError.Path)
}

// TestValidateNilContracts distinguishes required executors and definitions
// from optional limit overrides without needing a serialized resource first.
func TestValidateNilContracts(t *testing.T) {
	var definition *ToolDefinition
	var request *ProxyHTTP
	var limits *ExecutionLimits
	requireValidationPath(t, definition.Validate(nil), "$")
	requireValidationPath(t, request.Validate(&common.ValidationContext{Path: "spec.proxyHttp"}), "spec.proxyHttp")
	require.NoError(t, limits.Validate(nil))
	require.NoError(t, (&ExecutionLimits{}).Validate(nil))
}

// TestToolDefinitionValidateAccumulatesErrors retains all independent failures
// and their nested locations so authors can correct a definition in one pass.
func TestToolDefinitionValidateAccumulatesErrors(t *testing.T) {
	requests := 0
	definition := ToolDefinition{
		Description: " ",
		Verbs:       []string{" tool:write "},
		InputSchema: common.RawJSON(`{"type":"object"}`),
		Limits:      &ExecutionLimits{MaxRequests: &requests},
		ProxyHTTP: &ProxyHTTP{
			Method: "POST", URL: "https://example.test",
			Headers: map[string]string{"Bad Header": "x", "X-Tenant": "x\r\ny"},
		},
	}
	err := definition.Validate(&common.ValidationContext{Path: "spec"})
	var aggregate *multierror.Error
	require.ErrorAs(t, err, &aggregate)
	var paths []string
	for _, failure := range aggregate.Errors {
		var fieldError *common.ValidationError
		require.ErrorAs(t, failure, &fieldError)
		paths = append(paths, fieldError.Path)
	}
	require.ElementsMatch(t, []string{
		"spec.description", "spec.verbs[0]", "spec.limits.maxRequests",
		"spec.proxyHttp.headers.Bad Header", "spec.proxyHttp.headers.X-Tenant",
	}, paths)
	require.Equal(t, " tool:write ", definition.Verbs[0], "validation must not rewrite aliases")
	require.Zero(t, *definition.Limits.MaxRequests, "validation must not apply defaults")
}

// TestProxyHTTPValidateProgrammaticValues exercises the Go validation boundary
// directly, including invalid RawJSON that a JSON decoder would reject earlier.
func TestProxyHTTPValidateProgrammaticValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProxyHTTP)
		path   string
	}{
		{"header name", func(p *ProxyHTTP) { p.Headers = map[string]string{"Bad Header": "x"} }, "headers.Bad Header"},
		{"header carriage return", func(p *ProxyHTTP) { p.Headers = map[string]string{"X-Tenant": "x\ry"} }, "headers.X-Tenant"},
		{"header line feed", func(p *ProxyHTTP) { p.Headers = map[string]string{"X-Tenant": "x\ny"} }, "headers.X-Tenant"},
		{"header tokens", func(p *ProxyHTTP) { p.Headers = map[string]string{"X-Custom!#$%&'*+-.^_`|~": "{{cfg.tenant}}"} }, ""},
		{"malformed query JSON", func(p *ProxyHTTP) { p.Query = map[string]common.RawJSON{"q": common.RawJSON(`{`)} }, "query.q"},
		{"trailing query JSON", func(p *ProxyHTTP) { p.Query = map[string]common.RawJSON{"q": common.RawJSON(`1 2`)} }, "query.q"},
		{"query repeated lookup", func(p *ProxyHTTP) {
			p.Query = map[string]common.RawJSON{"q": common.RawJSON(`[null,false,1,"text",{"$value":"params.tag"}]`)}
		}, ""},
		{"query invalid array lookup", func(p *ProxyHTTP) {
			p.Query = map[string]common.RawJSON{"q": common.RawJSON(`["ok",{"$value":false}]`)}
		}, "query.q[1]"},
		{"query nested array", func(p *ProxyHTTP) {
			p.Query = map[string]common.RawJSON{"q": common.RawJSON(`["ok",[1]]`)}
		}, "query.q[1]"},
		{"missing form fields", func(p *ProxyHTTP) { p.Form = &FormBody{} }, "form.fields"},
		{"empty form fields", func(p *ProxyHTTP) { p.Form = &FormBody{Fields: map[string]common.RawJSON{}} }, ""},
		{"malformed form JSON", func(p *ProxyHTTP) {
			p.Form = &FormBody{Fields: map[string]common.RawJSON{"q": common.RawJSON(`true false`)}}
		}, "form.fields.q"},
		{"empty multipart", func(p *ProxyHTTP) { p.Multipart = &MultipartBody{} }, "multipart.parts"},
		{"malformed body JSON", func(p *ProxyHTTP) { p.BodyJSON = common.RawJSON(`{"x":`) }, "bodyJson"},
		{"trailing body JSON", func(p *ProxyHTTP) { p.BodyJSON = common.RawJSON(`null true`) }, "bodyJson"},
		{"valid array body", func(p *ProxyHTTP) { p.BodyJSON = common.RawJSON(`[false,{"$value":"params.item"}]`) }, ""},
		{"nested body directive", func(p *ProxyHTTP) {
			p.BodyJSON = common.RawJSON(`{"items":[true,{"$value":" "}]}`)
		}, "bodyJson.items[1].$value"},
		{"multi-key value is ordinary data", func(p *ProxyHTTP) {
			p.BodyJSON = common.RawJSON(`{"$value":null,"other":true}`)
		}, ""},
		{"multi-key literal is not an escape", func(p *ProxyHTTP) {
			p.BodyJSON = common.RawJSON(`{"$literal":{"$value":null},"other":true}`)
		}, "bodyJson.$literal.$value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := ProxyHTTP{Method: "POST", URL: "https://{{cfg.host}}/items"}
			test.change(&request)
			err := request.Validate(&common.ValidationContext{Path: "spec.proxyHttp"})
			if test.path == "" {
				require.NoError(t, err)
			} else {
				requireValidationPath(t, err, "spec.proxyHttp."+test.path)
			}
		})
	}
}

// TestMultipartPartValidateMetadata protects MIME header boundaries without
// treating filenames as server paths or requiring text parts to be nonempty.
func TestMultipartPartValidateMetadata(t *testing.T) {
	text := ""
	for _, test := range []struct {
		name, filename, mediaType string
		path                      string
	}{
		{"field", "report.csv", "text/csv; charset=utf-8", ""},
		{"field\r\nX-Injected: true", "", "", "name"},
		{"field", "report.csv\nX-Injected: true", "", "filename"},
		{"field", "report.csv", "text/plain; charset=\"unclosed", "mediaType"},
	} {
		t.Run(test.name+test.filename+test.mediaType, func(t *testing.T) {
			part := MultipartPart{Name: test.name, Filename: test.filename, MediaType: test.mediaType, Text: &text}
			err := part.Validate(&common.ValidationContext{Path: "spec.proxyHttp.multipart.parts[2]"})
			if test.path == "" {
				require.NoError(t, err)
			} else {
				requireValidationPath(t, err, "spec.proxyHttp.multipart.parts[2]."+test.path)
			}
		})
	}
}

// TestHTTPResponseValidateBoundaries checks the response rules directly so
// error paths and selector limits cannot rely on JSON Schema enforcement alone.
func TestHTTPResponseValidateBoundaries(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*HTTPResponse)
		path   string
	}{
		{"blank transform", func(r *HTTPResponse) { v := " \t"; r.TransformJavascript = &v }, "transformJavascript"},
		{"nonempty transform", func(r *HTTPResponse) { v := "response.body"; r.TransformJavascript = &v }, ""},
		{"empty statuses", func(r *HTTPResponse) { r.Errors[0].Statuses = []int{} }, "errors[0].statuses"},
		{"status below range", func(r *HTTPResponse) { r.Errors[0].Statuses = []int{99} }, "errors[0].statuses"},
		{"status endpoints", func(r *HTTPResponse) { r.Errors[0].Statuses = []int{100, 599} }, ""},
		{"blank code", func(r *HTTPResponse) { r.Errors[0].Code = " " }, "errors[0].code"},
		{"padded code", func(r *HTTPResponse) { r.Errors[0].Code = " ERROR " }, "errors[0].code"},
		{"class above range", func(r *HTTPResponse) { v := 6; r.Errors[0].Statuses = nil; r.Errors[0].StatusClass = &v }, "errors[0].statusClass"},
		{"class endpoints", func(r *HTTPResponse) {
			first, last := 1, 5
			r.Errors[0].Statuses = nil
			r.Errors[0].StatusClass = &first
			r.Errors = append(r.Errors, HTTPErrorRule{StatusClass: &last, Code: "LAST", Message: "Last class"})
		}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := HTTPResponse{Errors: []HTTPErrorRule{{Statuses: []int{404}, Code: "MISSING", Message: "Missing"}}}
			test.change(&response)
			err := response.Validate(&common.ValidationContext{Path: "spec.proxyHttp.response"})
			if test.path == "" {
				require.NoError(t, err)
			} else {
				requireValidationPath(t, err, "spec.proxyHttp.response."+test.path)
			}
		})
	}
}

// TestProxyHTTPValidateEncodingBoundaries checks both sides of the depth limit
// and strict base64 padding while allowing binary templates for later rendering.
func TestProxyHTTPValidateEncodingBoundaries(t *testing.T) {
	for _, depth := range []int{64, 65} {
		request := ProxyHTTP{Method: "POST", URL: "https://example.test", BodyJSON: common.RawJSON(strings.Repeat("[", depth) + "null" + strings.Repeat("]", depth))}
		err := request.Validate(nil)
		if depth == 64 {
			require.NoError(t, err)
		} else {
			requireValidationPath(t, err, "$.bodyJson")
		}
	}
	for _, test := range []struct {
		value string
		valid bool
	}{{"Zg==", true}, {"Zh==", false}, {"{{params.data}}", true}} {
		t.Run(test.value, func(t *testing.T) {
			request := ProxyHTTP{Method: "POST", URL: "https://example.test", BodyRaw: &test.value}
			err := request.Validate(nil)
			if test.valid {
				require.NoError(t, err)
			} else {
				requireValidationPath(t, err, "$.bodyRaw")
			}
		})
	}
}
