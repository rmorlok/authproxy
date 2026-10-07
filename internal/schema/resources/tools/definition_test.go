package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// definitionJSON gives executor fixtures a complete reusable definition without
// introducing connection identity or a resource envelope into this contract.
func definitionJSON(executor string) []byte {
	return []byte(`{"description":"Create a record","verbs":["tool:records.create"],"inputSchema":{"type":"object"},` + executor + `}`)
}

// TestToolDefinitionBodyModes verifies that every authored HTTP body form and
// the JavaScript executor survive the shared JSON/YAML input boundaries.
func TestToolDefinitionBodyModes(t *testing.T) {
	for name, executor := range map[string]string{
		"javascript":    `"javascript":"async function execute(params, context) { return params; }"`,
		"no body":       `"proxyHttp":{"method":"GET","url":"https://{{cfg.host}}/records"}`,
		"json":          `"proxyHttp":{"method":"POST","url":"https://example.test","bodyJson":{"enabled":{"$value":"params.enabled"},"literal":{"$literal":{"$value":"keep"}},"tag":"{{params.tag}}"}}`,
		"json null":     `"proxyHttp":{"method":"POST","url":"https://example.test","bodyJson":null}`,
		"template":      `"proxyHttp":{"method":"POST","url":"https://example.test","bodyTemplate":{"mediaType":"application/json","template":"{\"enabled\": {{json.params.enabled}}}"}}`,
		"raw":           `"proxyHttp":{"method":"POST","url":"https://example.test","headers":{"Content-Type":"application/octet-stream"},"bodyRaw":"AAEC"}`,
		"empty raw":     `"proxyHttp":{"method":"POST","url":"https://example.test","bodyRaw":""}`,
		"form":          `"proxyHttp":{"method":"POST","url":"https://example.test","form":{"fields":{"enabled":false,"tag":["one","two"],"account":{"$value":"cfg.account"}}}}`,
		"multipart":     `"proxyHttp":{"method":"POST","url":"https://example.test","multipart":{"parts":[{"name":"title","text":"{{params.title}}"},{"name":"file","filename":"report.bin","mediaType":"application/octet-stream","bodyRaw":"AAEC"}]}}`,
		"custom method": `"proxyHttp":{"method":"PROPFIND","url":"https://example.test"}`,
	} {
		t.Run(name, func(t *testing.T) {
			input := definitionJSON(executor)
			var definition ToolDefinition
			require.NoError(t, util.DecodeJSONStrict(input, &definition))
			require.NoError(t, definition.Validate(nil))
			encoded, err := json.Marshal(definition)
			require.NoError(t, err)
			require.JSONEq(t, string(input), string(encoded))

			asYAML, err := yaml.Marshal(definition)
			require.NoError(t, err)
			var fromYAML ToolDefinition
			require.NoError(t, util.DecodeYAMLStrict(asYAML, &fromYAML))
			require.NoError(t, fromYAML.Validate(nil))
			encoded, err = json.Marshal(fromYAML)
			require.NoError(t, err)
			require.JSONEq(t, string(input), string(encoded))
		})
	}
}

// TestToolDefinitionRejectsConflictingOrIncompleteExecutors ensures invalid
// authored operations fail before a compiler or connection can act on them.
func TestToolDefinitionRejectsConflictingOrIncompleteExecutors(t *testing.T) {
	for name, executor := range map[string]string{
		"missing executor":         `"hints":{}`,
		"both executors":           `"javascript":"code","proxyHttp":{"method":"GET","url":"https://example.test"}`,
		"empty javascript":         `"javascript":"  "`,
		"missing method":           `"proxyHttp":{"url":"https://example.test"}`,
		"invalid method":           `"proxyHttp":{"method":"GET\r\nX-Test: true","url":"https://example.test"}`,
		"missing url":              `"proxyHttp":{"method":"GET"}`,
		"competing body modes":     `"proxyHttp":{"method":"POST","url":"https://example.test","bodyJson":null,"bodyRaw":""}`,
		"template media type":      `"proxyHttp":{"method":"POST","url":"https://example.test","bodyTemplate":{"template":"hello"}}`,
		"invalid base64":           `"proxyHttp":{"method":"POST","url":"https://example.test","bodyRaw":"this is not base64"}`,
		"nested query object":      `"proxyHttp":{"method":"GET","url":"https://example.test","query":{"filter":{"a":1}}}`,
		"nested form array":        `"proxyHttp":{"method":"POST","url":"https://example.test","form":{"fields":{"key":[[1]]}}}`,
		"unnamed part":             `"proxyHttp":{"method":"POST","url":"https://example.test","multipart":{"parts":[{"text":"hello"}]}}`,
		"empty part":               `"proxyHttp":{"method":"POST","url":"https://example.test","multipart":{"parts":[{"name":"file"}]}}`,
		"competing part modes":     `"proxyHttp":{"method":"POST","url":"https://example.test","multipart":{"parts":[{"name":"file","text":"","bodyRaw":""}]}}`,
		"invalid multipart base64": `"proxyHttp":{"method":"POST","url":"https://example.test","multipart":{"parts":[{"name":"file","bodyRaw":"!"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var definition ToolDefinition
			require.NoError(t, util.DecodeJSONStrict(definitionJSON(executor), &definition))
			require.Error(t, definition.Validate(nil))
		})
	}
}

// TestToolDefinitionStrictAuthoring keeps compiler plans and misspelled owned
// fields out of authored definitions while provider payloads remain opaque.
func TestToolDefinitionStrictAuthoring(t *testing.T) {
	for name, executor := range map[string]string{
		"openapi plan":           `"openapiOperation":{}`,
		"mcp plan":               `"mcpCall":{}`,
		"unknown executor field": `"proxyHttp":{"method":"GET","url":"https://example.test","hostOverride":"example.test"}`,
		"unknown rule field":     `"proxyHttp":{"method":"GET","url":"https://example.test","response":{"errors":[{"statuses":[404],"code":"NOT_FOUND","message":"Missing","unknown":true}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			input := definitionJSON(executor)
			var definition ToolDefinition
			require.Error(t, util.DecodeJSONStrict(input, &definition))
			var generic any
			require.NoError(t, json.Unmarshal(input, &generic))
			asYAML, err := yaml.Marshal(generic)
			require.NoError(t, err)
			require.Error(t, util.DecodeYAMLStrict(asYAML, &definition))
		})
	}
}

// TestToolDefinitionErrorRules allows precedence across selector classes while
// rejecting ambiguity within one class, independent of slice order.
func TestToolDefinitionErrorRules(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules string
		valid bool
	}{
		{"precedence", `[{"statuses":[404],"code":"MISSING","message":"Missing"},{"statusClass":4,"code":"CLIENT","message":"Client error"},{"default":true,"code":"UPSTREAM","message":"Failed"}]`, true},
		{"same rule duplicate", `[{"statuses":[404,404],"code":"MISSING","message":"Missing"}]`, false},
		{"duplicate status", `[{"statuses":[404],"code":"A","message":"A"},{"statuses":[404],"code":"B","message":"B"}]`, false},
		{"duplicate class", `[{"statusClass":4,"code":"A","message":"A"},{"statusClass":4,"code":"B","message":"B"}]`, false},
		{"duplicate default", `[{"default":true,"code":"A","message":"A"},{"default":true,"code":"B","message":"B"}]`, false},
		{"multiple selectors", `[{"statuses":[404],"default":true,"code":"A","message":"A"}]`, false},
		{"missing selector", `[{"code":"A","message":"A"}]`, false},
		{"invalid status", `[{"statuses":[600],"code":"A","message":"A"}]`, false},
		{"invalid class", `[{"statusClass":0,"code":"A","message":"A"}]`, false},
		{"missing message", `[{"statuses":[404],"code":"A"}]`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := definitionJSON(`"proxyHttp":{"method":"GET","url":"https://example.test","response":{"errors":` + test.rules + `}}`)
			var definition ToolDefinition
			require.NoError(t, util.DecodeJSONStrict(input, &definition))
			err := definition.Validate(nil)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "response.errors")
			}
		})
	}
}

// TestToolDefinitionHintsAndLimits retain explicit false hints and exact byte
// and time limits instead of rounding them through human-readable unit strings.
func TestToolDefinitionHintsAndLimits(t *testing.T) {
	input := definitionJSON(`"javascript":"code","hints":{"readOnly":false,"destructive":true,"idempotent":false},"limits":{"timeoutMillis":1500,"maxRequests":2,"maxInputBytes":1536,"maxOutputBytes":2048,"maxResponseBytes":4096,"maxEncodedResultBytes":8192,"maxStackDepth":32},"outputSchema":false`)
	var definition ToolDefinition
	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		require.NoError(t, decode(input, &definition)) // JSON is also valid YAML.
		require.NoError(t, definition.Validate(nil))
		encoded, err := json.Marshal(definition)
		require.NoError(t, err)
		require.JSONEq(t, string(input), string(encoded))
	}
	for _, field := range []string{"timeoutMillis", "maxRequests", "maxInputBytes", "maxOutputBytes", "maxResponseBytes", "maxEncodedResultBytes", "maxStackDepth"} {
		for _, value := range []string{"0", "-1"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				var definition ToolDefinition
				require.NoError(t, util.DecodeJSONStrict(definitionJSON(`"javascript":"code","limits":{"`+field+`":`+value+`}`), &definition))
				require.ErrorContains(t, definition.Validate(nil), field)
			})
		}
	}
}

// TestToolDefinitionRequiredMetadata guards the aliases and agent-facing
// description independently from whichever executor will eventually compile.
func TestToolDefinitionRequiredMetadata(t *testing.T) {
	for _, test := range []struct{ old, replacement, field string }{
		{`"description":"Create a record"`, `"description":" "`, "description"},
		{`["tool:records.create"]`, `[]`, "verbs"},
		{`["tool:records.create"]`, `[""]`, "verbs"},
		{`["tool:records.create"]`, `["tool:a","tool:a"]`, "verbs"},
	} {
		t.Run(test.replacement, func(t *testing.T) {
			input := strings.Replace(string(definitionJSON(`"javascript":"code"`)), test.old, test.replacement, 1)
			var definition ToolDefinition
			require.NoError(t, util.DecodeJSONStrict([]byte(input), &definition))
			require.ErrorContains(t, definition.Validate(nil), test.field)
		})
	}
}

// TestProxyHTTPYAMLAliasesPreserveBodyPresence prevents YAML aliases and merge
// keys from silently removing a JSON null body or hiding a competing mode.
func TestProxyHTTPYAMLAliasesPreserveBodyPresence(t *testing.T) {
	for _, test := range []struct {
		name, source string
		valid        bool
	}{
		{"alias JSON null", "method: POST\nurl: https://example.test\nquery: {unused: &nil null}\nbodyJson: *nil\n", true},
		{"merge JSON null", "<<: {method: POST, url: 'https://example.test', bodyJson: null}\n", true},
		{"alias competing body", "method: POST\nurl: https://example.test\nquery: {unused: &nil null}\nbodyJson: *nil\nbodyRaw: ''\n", false},
		{"merge competing body", "<<: {method: POST, url: 'https://example.test', bodyJson: null}\nbodyRaw: ''\n", false},
		{"alias invalid raw null", "method: POST\nurl: https://example.test\nquery: {unused: &nil null}\nbodyRaw: *nil\n", false},
		{"merge invalid form null", "<<: {method: POST, url: 'https://example.test', form: null}\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var request ProxyHTTP
			err := util.DecodeYAMLStrict([]byte(test.source), &request)
			if err == nil {
				err = request.Validate(nil)
			}
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, "null", string(request.BodyJSON))
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestProxyHTTPRejectsErasedWireFields checks fields for which zero values are
// valid but omission or explicit null would change the requested operation.
func TestProxyHTTPRejectsErasedWireFields(t *testing.T) {
	for _, body := range []string{
		`"bodyTemplate":{"mediaType":"text/plain"}`,
		`"bodyTemplate":{"mediaType":"text/plain","template":null}`,
		`"multipart":{"parts":[{"name":"x","text":null,"bodyRaw":""}]}`,
		`"multipart":{"parts":[{"name":"x","text":"","bodyRaw":null}]}`,
		`"response":{"errors":[{"statuses":null,"default":true,"code":"A","message":"A"}]}`,
		`"response":{"errors":[{"statusClass":null,"default":true,"code":"A","message":"A"}]}`,
		`"response":{"errors":[{"statuses":[404],"default":false,"code":"A","message":"A"}]}`,
	} {
		input := []byte(`{"method":"POST","url":"https://example.test",` + body + `}`)
		for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
			var request ProxyHTTP
			require.Error(t, decode(input, &request), body)
		}
	}
}

// TestProxyHTTPLiteralEscapePreservesDataButRetainsDepthBound ensures escaping
// a directive does not also escape the recursive input bound.
func TestProxyHTTPLiteralEscapePreservesDataButRetainsDepthBound(t *testing.T) {
	for _, test := range []struct {
		body  string
		valid bool
	}{
		{`{"$literal":{"$value":null}}`, true},
		{`{"$value":null}`, false},
		{`{"$literal":` + strings.Repeat("[", 70) + `null` + strings.Repeat("]", 70) + `}`, false},
	} {
		var request ProxyHTTP
		require.NoError(t, util.DecodeJSONStrict([]byte(`{"method":"POST","url":"https://example.test","bodyJson":`+test.body+`}`), &request))
		err := request.Validate(nil)
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

// TestProxyHTTPValidationPreservesJSONNumbers prevents contract validation from
// imposing float64's range or precision on authored JSON request values.
func TestProxyHTTPValidationPreservesJSONNumbers(t *testing.T) {
	for name, body := range map[string]string{
		"query":     `"query":{"value":1e400}`,
		"form":      `"form":{"fields":{"value":1e400}}`,
		"JSON body": `"bodyJson":{"value":1e400}`,
	} {
		t.Run(name, func(t *testing.T) {
			var request ProxyHTTP
			require.NoError(t, util.DecodeJSONStrict([]byte(`{"method":"POST","url":"https://example.test",`+body+`}`), &request))
			require.NoError(t, request.Validate(nil))
			encoded, err := json.Marshal(request)
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"value":1e400`)
		})
	}
}
