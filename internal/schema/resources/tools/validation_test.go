package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	requireValidationPath(t, request.ValidateBody(&common.ValidationContext{Path: "spec.proxyHttp"}), "spec.proxyHttp")
	require.NoError(t, limits.Validate(nil))
	require.NoError(t, (&ExecutionLimits{}).Validate(nil))
}

// TestProxyHTTPValidateBodyIgnoresOtherRequestFields allows body compilation
// independently of the request envelope and response mapping.
func TestProxyHTTPValidateBodyIgnoresOtherRequestFields(t *testing.T) {
	// Every non-body component is invalid, but the explicit JSON body is valid.
	blank := ""
	request := ProxyHTTP{
		Headers:  map[string]string{"Bad Header": "x"},
		Query:    map[string]common.RawJSON{"q": common.RawJSON(`{`)},
		BodyJSON: common.RawJSON(`null`),
		Response: &HTTPResponse{TransformJavascript: &blank},
	}
	require.NoError(t, request.ValidateBody(nil))

	// Full validation continues to report every unrelated failure.
	err := request.Validate(nil)
	var aggregate *multierror.Error
	require.ErrorAs(t, err, &aggregate)
	var paths []string
	for _, failure := range aggregate.Errors {
		var fieldError *common.ValidationError
		require.ErrorAs(t, failure, &fieldError)
		paths = append(paths, fieldError.Path)
	}
	require.ElementsMatch(t, []string{
		"$.method", "$.url", "$.headers.Bad Header", "$.query.q", "$.response.transformJavascript",
	}, paths)
}

// TestProxyHTTPValidateBodyDecodedPresence preserves omitted bodies and JSON
// null while rejecting explicit null for all other body modes in both codecs.
func TestProxyHTTPValidateBodyDecodedPresence(t *testing.T) {
	for _, codec := range []string{"JSON", "YAML"} {
		for _, test := range []struct {
			name, document, path string
			jsonBody             bool
		}{
			{"omitted body", `{}`, "", false},
			{"JSON null body", `{"bodyJson":null}`, "", true},
			{"null template", `{"bodyTemplate":null}`, "bodyTemplate", false},
			{"null raw body", `{"bodyRaw":null}`, "bodyRaw", false},
			{"null form", `{"form":null}`, "form", false},
			{"null multipart", `{"multipart":null}`, "multipart", false},
		} {
			t.Run(codec+"/"+test.name, func(t *testing.T) {
				// JSON syntax is also valid YAML, exercising each decoder's presence handling.
				var request ProxyHTTP
				if codec == "JSON" {
					require.NoError(t, json.Unmarshal([]byte(test.document), &request))
				} else {
					require.NoError(t, yaml.Unmarshal([]byte(test.document), &request))
				}

				// Body validation uses the original decoded object, including private null markers.
				err := request.ValidateBody(&common.ValidationContext{Path: "spec.proxyHttp"})
				if test.path != "" {
					requireValidationPath(t, err, "spec.proxyHttp."+test.path)
					return
				}
				require.NoError(t, err)
				if test.jsonBody {
					require.JSONEq(t, "null", string(request.BodyJSON))
				} else {
					require.Nil(t, request.BodyJSON)
				}
			})
		}
	}
}

// TestProxyHTTPValidateBodyModes checks each mode alone and every conflicting
// pair without requiring a method or URL for the body-only validation boundary.
func TestProxyHTTPValidateBodyModes(t *testing.T) {
	empty := ""
	modes := []struct {
		name  string
		apply func(*ProxyHTTP)
	}{
		{"JSON", func(p *ProxyHTTP) { p.BodyJSON = common.RawJSON(`null`) }},
		{"template", func(p *ProxyHTTP) { p.BodyTemplate = &BodyTemplate{MediaType: "text/plain"} }},
		{"raw", func(p *ProxyHTTP) { p.BodyRaw = &empty }},
		{"form", func(p *ProxyHTTP) { p.Form = &FormBody{Fields: map[string]common.RawJSON{}} }},
		{"multipart", func(p *ProxyHTTP) {
			p.Multipart = &MultipartBody{Parts: []MultipartPart{{Name: "field", Text: &empty}}}
		}},
	}
	for i, first := range modes {
		t.Run(first.name, func(t *testing.T) {
			// A single complete mode is valid even when its content is empty or null.
			var request ProxyHTTP
			first.apply(&request)
			require.NoError(t, request.ValidateBody(nil))
		})
		for _, second := range modes[i+1:] {
			t.Run(first.name+" and "+second.name, func(t *testing.T) {
				// No pair may silently select one mode and discard the other.
				var request ProxyHTTP
				first.apply(&request)
				second.apply(&request)
				err := request.ValidateBody(&common.ValidationContext{Path: "spec.proxyHttp"})
				requireValidationPath(t, err, "spec.proxyHttp")
				require.ErrorContains(t, err, "at most one")
			})
		}
	}
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

// TestProxyHTTPOptionalParameterContracts checks native and embedded-schema
// validation agree on complete-field optional lookups and nonempty names.
func TestProxyHTTPOptionalParameterContracts(t *testing.T) {
	compiled, err := schema.CompileSchema(SchemaIDTools + "#/$defs/ProxyHTTP")
	require.NoError(t, err)
	for _, mode := range []string{"query", "form"} {
		for _, tc := range []struct {
			name string
			key  string
			raw  string
			fail string
		}{
			{"optional lookup", "cursor", `{"$optional":"params.cursor"}`, ""},
			{"required lookup unchanged", "cursor", `{"$value":"params.cursor"}`, ""},
			{"literal null unchanged", "cursor", `null`, ""},
			{"repeated typed lookup unchanged", "cursor", `[false,null,{"$value":"params.cursor"}]`, ""},
			{"empty reference", "cursor", `{"$optional":""}`, ".cursor"},
			{"blank reference", "cursor", `{"$optional":" \t"}`, ".cursor"},
			{"null reference", "cursor", `{"$optional":null}`, ".cursor"},
			{"boolean reference", "cursor", `{"$optional":false}`, ".cursor"},
			{"array reference", "cursor", `{"$optional":["params.cursor"]}`, ".cursor"},
			{"mixed optional object", "cursor", `{"$optional":"params.cursor","other":true}`, ".cursor"},
			{"mixed directives", "cursor", `{"$optional":"params.cursor","$value":"params.cursor"}`, ".cursor"},
			{"optional array entry", "cursor", `[{"$optional":"params.cursor"}]`, ".cursor[0]"},
			{"empty name", "", `{"$optional":"params.cursor"}`, "field map"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				// Both authoring locations share one field-value contract.
				fields := map[string]common.RawJSON{tc.key: common.RawJSON(tc.raw)}
				request := ProxyHTTP{Method: "POST", URL: "https://example.test/items"}
				path := "$.query"
				if mode == "query" {
					request.Query = fields
				} else {
					request.Form = &FormBody{Fields: fields}
					path = "$.form.fields"
				}
				nativeErr := request.Validate(nil)

				// Validate the serialized shape separately from the native boundary.
				encoded, err := json.Marshal(request)
				require.NoError(t, err)
				var document any
				require.NoError(t, json.Unmarshal(encoded, &document))
				schemaErr := compiled.Validate(document)
				if tc.fail == "" {
					require.NoError(t, nativeErr)
					require.NoError(t, schemaErr)
					return
				}
				if tc.fail != "field map" {
					path += tc.fail
				}
				requireValidationPath(t, nativeErr, path)
				require.Error(t, schemaErr)
			})
		}
	}
}

// TestProxyHTTPParameterPresenceRoundTrip preserves optional directives and
// explicit values through JSON and YAML without applying runtime omission.
func TestProxyHTTPParameterPresenceRoundTrip(t *testing.T) {
	// Required, optional, and explicit-null values must remain distinct in storage.
	fields := map[string]common.RawJSON{
		"optional": common.RawJSON(`{"$optional":"params.cursor"}`),
		"required": common.RawJSON(`{"$value":"params.cursor"}`),
		"null":     common.RawJSON(`null`),
		"empty":    common.RawJSON(`""`),
		"false":    common.RawJSON(`false`),
		"zero":     common.RawJSON(`0`),
		"repeated": common.RawJSON(`[]`),
	}
	request := ProxyHTTP{Method: "POST", URL: "https://example.test/items", Query: fields, Form: &FormBody{Fields: fields}}
	require.NoError(t, request.Validate(nil))
	expected, err := json.Marshal(request)
	require.NoError(t, err)
	var fromJSON ProxyHTTP
	require.NoError(t, util.DecodeJSONStrict(expected, &fromJSON))
	require.NoError(t, fromJSON.Validate(nil))
	roundTrip, err := json.Marshal(fromJSON)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(roundTrip))

	// YAML's null handling must not erase explicit field values or directives.
	yamlBytes, err := yaml.Marshal(request)
	require.NoError(t, err)
	var fromYAML ProxyHTTP
	require.NoError(t, util.DecodeYAMLStrict(yamlBytes, &fromYAML))
	require.NoError(t, fromYAML.Validate(nil))
	roundTrip, err = json.Marshal(fromYAML)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(roundTrip))
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
		{"資料", "résumé-雪.csv", `Text/CSV; charset="UTF-8"`, ""},
		{"{{params.name}}", "{{cfg.filename}}", `text/plain; note="{{cfg.literal}}"`, ""},
		{"{{\tparams.name\t}}", "{{\tparams.filename\t}}", "", ""},
		{"field", "", "", ""},
		{"", "report.csv", "", "name"},
		{"   ", "report.csv", "", "name"},
		{"field\r\nX-Injected: true", "", "", "name"},
		{"field", "report.csv\nX-Injected: true", "", "filename"},
		{"{{params.name}}\n", "", "", "name"},
		{"field", "{{params.filename}}\r", "", "filename"},
		{"field\xff", "", "", "name"},
		{"field", "report\xff.csv", "", "filename"},
		{"{{params.name}}\xff", "", "", "name"},
		{"field", "{{params.filename}}\xff", "", "filename"},
		{"field", "report.csv", "text/plain; charset=\"unclosed", "mediaType"},
	} {
		t.Run(test.name+test.filename+test.mediaType, func(t *testing.T) {
			// Static fields and template syntax are checked without rendering inputs.
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

// TestMultipartMetadataControls rejects every ASCII control in literal names
// and filenames both directly and through either HTTP validation boundary.
func TestMultipartMetadataControls(t *testing.T) {
	text := ""
	for code := byte(0); code <= 0x7f; code++ {
		if code >= ' ' && code != 0x7f {
			continue
		}
		for _, field := range []string{"name", "filename"} {
			t.Run(fmt.Sprintf("%s/ASCII-%02x", field, code), func(t *testing.T) {
				// The shared resolved-value predicate rejects the entire C0/DEL range.
				value := "private-metadata" + string(code)
				require.False(t, IsValidMultipartMetadata(value))
				part := MultipartPart{Name: "field", Filename: "report.txt", Text: &text}
				if field == "name" {
					part.Name = value
				} else {
					part.Filename = value
				}

				// Admission reports the same field for direct, body-only, and full requests.
				requireValidationPath(t, part.Validate(&common.ValidationContext{Path: "part"}), "part."+field)
				request := ProxyHTTP{Method: "POST", URL: "https://example.test", Multipart: &MultipartBody{Parts: []MultipartPart{part}}}
				for _, validate := range []func(*common.ValidationContext) error{request.Validate, request.ValidateBody} {
					err := validate(&common.ValidationContext{Path: "spec.proxyHttp"})
					requireValidationPath(t, err, "spec.proxyHttp.multipart.parts[0]."+field)
					require.NotContains(t, err.Error(), "private-metadata")
				}
			})
		}
	}

	// Empty optional filenames and Unicode/template source remain valid metadata.
	for _, value := range []string{"", "résumé-雪.csv", `quote"and\slash`, "{{params.filename}}"} {
		require.True(t, IsValidMultipartMetadata(value))
	}
	require.False(t, IsValidMultipartMetadata("filename\xff"))
	// Tabs may belong to authored Mustache syntax but never to resolved MIME values.
	require.False(t, IsValidMultipartMetadata("{{\tparams.filename\t}}"))
}

// TestBodyMediaTypeValidation uses the same concrete-media contract at schema
// admission and runtime selection, including parameterized and uppercase types.
func TestBodyMediaTypeValidation(t *testing.T) {
	for _, test := range []struct {
		name, value, base string
	}{
		{"JSON", "Application/JSON; charset=utf-8", "application/json"},
		{"JSON suffix", "application/problem+json", "application/problem+json"},
		{"quoted parameter", `Text/Plain; charset="UTF-8"`, "text/plain"},
		{"literal template parameter", `text/plain; note="{{cfg.mime}}"`, "text/plain"},
		{"missing type", "", ""},
		{"bare token", "private-metadata", ""},
		{"missing subtype", "text/", ""},
		{"missing major type", "/json", ""},
		{"wildcard subtype", "text/*", ""},
		{"wildcard major type", "*/json", ""},
		{"wildcard suffix", "application/*+json", ""},
		{"bad parameter", "text/plain; broken", ""},
		{"tab", "text/plain\t", ""},
		{"NUL", "text/plain\x00", ""},
		{"DEL", "text/plain\x7f", ""},
		{"newlines", "text/plain\r\nprivate-metadata", ""},
		{"invalid UTF-8", "text/plain\xff", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Parsing normalizes only the base media type and never reveals bad inputs.
			base, err := ParseBodyMediaType(test.value)
			require.Equal(t, test.base, base)
			if test.base == "" {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-metadata")
			} else {
				require.NoError(t, err)
			}

			// Both HTTP body paths and direct multipart validation use the same policy.
			template := &BodyTemplate{MediaType: test.value, Template: ""}
			text := ""
			part := MultipartPart{Name: "field", MediaType: test.value, Text: &text}
			for _, mode := range []string{"bodyTemplate", "multipart"} {
				request := ProxyHTTP{Method: "POST", URL: "https://example.test"}
				path := "spec.proxyHttp.bodyTemplate.mediaType"
				valid := test.base != ""
				if mode == "bodyTemplate" {
					request.BodyTemplate = template
				} else {
					request.Multipart = &MultipartBody{Parts: []MultipartPart{part}}
					path = "spec.proxyHttp.multipart.parts[0].mediaType"
					valid = valid || test.value == "" // Omitted part media types use encoder defaults.
					partErr := part.Validate(&common.ValidationContext{Path: "part"})
					if valid {
						require.NoError(t, partErr)
					} else {
						requireValidationPath(t, partErr, "part.mediaType")
					}
				}
				for _, validate := range []func(*common.ValidationContext) error{request.Validate, request.ValidateBody} {
					err := validate(&common.ValidationContext{Path: "spec.proxyHttp"})
					if valid {
						require.NoError(t, err)
					} else {
						requireValidationPath(t, err, path)
						require.NotContains(t, err.Error(), "private-metadata")
					}
				}
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
