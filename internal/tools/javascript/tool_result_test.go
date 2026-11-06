package javascript_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// evaluateRichResult evaluates a Tool against an empty captured input and checks its result shape.
func evaluateRichResult(t *testing.T, program *javascript.ToolProgram) *javascript.Result {
	t.Helper()
	result, err := program.Evaluate(context.Background(), fetchInput(t))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Content)
	return result
}

// TestToolResultOrdinaryReturns preserves JSON values without interpreting business objects as envelopes.
func TestToolResultOrdinaryReturns(t *testing.T) {
	for _, value := range []string{
		`null`, `false`, `0`, `[1,"two"]`, `{"id":"customer-7"}`,
		`{"content":[{"type":"text","text":"business data"}],"structuredContent":null}`,
	} {
		t.Run(value, func(t *testing.T) {
			// Only calling the helper opts into explicit rich-result interpretation.
			program := compileTool(t, "async function execute() { return "+value+"; }", nil)
			result := evaluateRichResult(t, program)
			require.JSONEq(t, value, string(result.StructuredContent))
			require.Empty(t, result.Content)
			require.Nil(t, result.Provider)
		})
	}

	// A normal string remains structured JSON and additionally supplies a text block.
	result := evaluateRichResult(t, compileTool(t, `async function execute() { return "Saved\n✓"; }`, nil))
	require.JSONEq(t, `"Saved\n✓"`, string(result.StructuredContent))
	require.Len(t, result.Content, 1)
	require.Equal(t, api.ToolContentTypeText, result.Content[0].Type)
	require.Equal(t, "Saved\n✓", *result.Content[0].Text)
}

// TestToolResultOptionalValues distinguishes omitted structured data from explicit JSON null.
func TestToolResultOptionalValues(t *testing.T) {
	for _, test := range []struct {
		name, payload, structured string
	}{
		{"empty", `{}`, ""},
		{"explicit content", `{content: []}`, ""},
		{"null", `{structuredContent: null}`, `null`},
		{"false", `{structuredContent: false}`, `false`},
		{"string", `{structuredContent: "metadata only"}`, `"metadata only"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Explicit helpers never synthesize a text block from structuredContent.
			program := compileTool(t, "async function execute() { return toolResult("+test.payload+"); }", nil)
			result := evaluateRichResult(t, program)
			require.Empty(t, result.Content)
			if test.structured == "" {
				require.Nil(t, result.StructuredContent)
			} else {
				require.JSONEq(t, test.structured, string(result.StructuredContent))
			}
		})
	}
}

// TestToolResultPreservesRichBlocks retains union variants, rendering hints, and opaque provider data.
func TestToolResultPreservesRichBlocks(t *testing.T) {
	const blocks = `[
		{"type":"text","text":"","annotations":{"audience":["assistant"],"priority":0.75},"provider":{"format":"plain"}},
		{"type":"image","mimeType":"image/png","data":"AAEC"},
		{"type":"audio","mimeType":"audio/wav","data":"AwQ="},
		{"type":"resource_link","uri":"https://provider.example/report","name":"report","title":"Report","size":7,"icons":[{"src":"https://provider.example/icon.png","mimeType":"image/png"}]},
		{"type":"resource","resource":{"uri":"provider:notes","mimeType":"text/plain","text":"notes"}},
		{"type":"resource","resource":{"uri":"provider:blob","mimeType":"application/octet-stream","blob":"BQY="}}
	]`
	program := compileTool(t, `async function execute() {
		return toolResult({content: `+blocks+`, structuredContent: {id: "report-7"}, provider: {requestId: "req-7"}});
	}`, nil)

	// Content validates without fetching any referenced URI or rewriting metadata.
	result := evaluateRichResult(t, program)
	require.Len(t, result.Content, 6)
	for i := range result.Content {
		require.NoError(t, result.Content[i].Validate(nil))
	}
	encoded, err := json.Marshal(result.Content)
	require.NoError(t, err)
	require.JSONEq(t, blocks, string(encoded))
	require.JSONEq(t, `{"id":"report-7"}`, string(result.StructuredContent))
	require.JSONEq(t, `{"requestId":"req-7"}`, string(result.Provider))
}

// TestToolResultRejectsInvalidPayloads keeps malformed rich output and private diagnostics out of results.
func TestToolResultRejectsInvalidPayloads(t *testing.T) {
	for name, value := range map[string]string{
		"null content":      `toolResult({content: null})`,
		"nonarray content":  `toolResult({content: {}})`,
		"null block":        `toolResult({content: [null]})`,
		"unknown block":     `toolResult({content: [{type: "unknown"}]})`,
		"unknown block key": `toolResult({content: [{type: "text", text: "ok", private: true}]})`,
		"missing text":      `toolResult({content: [{type: "text"}]})`,
		"mixed variant":     `toolResult({content: [{type: "text", text: "private", data: "AA=="}]})`,
		"invalid base64":    `toolResult({content: [{type: "image", mimeType: "image/png", data: "private"}]})`,
		"relative resource": `toolResult({content: [{type: "resource_link", uri: "private/path", name: "file"}]})`,
		"invalid priority":  `toolResult({content: [{type: "text", text: "ok", annotations: {priority: 2}}]})`,
		"null provider":     `toolResult({provider: null})`,
		"array provider":    `toolResult({provider: []})`,
		"unknown field":     `toolResult({private: true})`,
		"tool identity":     `toolResult({toolId: ""})`,
		"revision":          `toolResult({revision: 0})`,
		"invocation":        `toolResult({invocationId: ""})`,
		"metadata":          `toolResult({metadata: {}})`,
		"error":             `toolResult({error: null})`,
		"error flag":        `toolResult({isError: false})`,
		"execution state":   `toolResult({executionState: "completed"})`,
		"nonfinite data":    `toolResult({structuredContent: NaN})`,
		"cyclic data":       `toolResult({structuredContent: (() => {const v = {}; v.self = v; return v;})()})`,
		"getter":            `toolResult({get content() { throw "private"; }})`,
		"proxy":             `toolResult(new Proxy({}, {ownKeys() { throw "private"; }}))`,
		"toJSON":            `toolResult({toJSON() { throw "private"; }})`,
		"nested helper":     `({value: toolResult({})})`,
		"forged helper":     `Object.create(Object.getPrototypeOf(toolResult({})))`,
		"proxied helper":    `new Proxy(toolResult({}), {})`,
		"lone text unit":    `toolResult({content: [{type: "text", text: "\ud800"}]})`,
		"lone plain unit":   `"\ud800"`,
	} {
		t.Run(name, func(t *testing.T) {
			// Invalid content must fail atomically without copying validation or getter text.
			program := compileTool(t, "async function execute() { return "+value+"; }", nil)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, javascript.ErrInvalidOutput)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}
}

// TestToolResultBoundsWholePayload applies one output allowance across every rich field.
func TestToolResultBoundsWholePayload(t *testing.T) {
	for _, test := range []struct {
		name, source string
		change       func(*javascript.Limits)
	}{
		{"bytes", `toolResult({content: [{type: "text", text: "x".repeat(128)}], provider: {id: "p"}})`, func(l *javascript.Limits) { l.MaxOutputBytes = 128 }},
		{"nodes", `toolResult({structuredContent: Array(100).fill(0)})`, func(l *javascript.Limits) { l.MaxNodes = 50 }},
		{"depth", `toolResult({structuredContent: {a: {b: {c: {d: {e: 1}}}}}})`, func(l *javascript.Limits) { l.MaxDepth = 4 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := expressionLimits()
			test.change(&limits)
			program, err := javascript.CompileTool("async function execute() { return "+test.source+"; }", nil, limits)
			require.NoError(t, err)

			// Limits reject the complete result rather than returning truncated blocks or JSON.
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, javascript.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestToolResultHelperValidationAndIsolation permits publication-time construction without adding pure-expression hosts.
func TestToolResultHelperValidationAndIsolation(t *testing.T) {
	program := compileTool(t, `
		const ready = toolResult({content: [{type: "text", text: "ready"}]});
		async function execute() { return ready; }
	`, nil)

	// The pure helper can run during initialization without a connection or credentials.
	require.NoError(t, program.Validate(context.Background()))
	require.Equal(t, "ready", *evaluateRichResult(t, program).Content[0].Text)

	// Response-transform expressions still have no Tool result helper or network host.
	expression := compileExpression(t, `({helper: typeof toolResult, network: typeof fetch})`)
	encoded := evaluateExpression(t, expression, javascript.Input{Context: expressionContext(t, render.Input{})})
	require.JSONEq(t, `{"helper":"undefined","network":"undefined"}`, string(encoded))
}

// TestToolResultCapturesIntrinsics keeps branding and conversion independent of mutable globals.
func TestToolResultCapturesIntrinsics(t *testing.T) {
	program := compileTool(t, `async function execute() {
		WeakMap.prototype.get = () => ({structuredContent: "forged"});
		WeakMap.prototype.has = () => false;
		WeakMap.prototype.set = () => { throw "private"; };
		Object.create = () => { throw "private"; };
		Object.freeze = () => { throw "private"; };
		JSON.stringify = () => "private";
		return toolResult({structuredContent: {saved: true}});
	}`, nil)

	// Captured native helpers still recognize the wrapper and preserve the actual payload.
	result := evaluateRichResult(t, program)
	require.JSONEq(t, `{"saved":true}`, string(result.StructuredContent))
	require.Empty(t, result.Content)
}

// TestToolResultOwnership detaches raw JSON, content pointers, and nested metadata across evaluations.
func TestToolResultOwnership(t *testing.T) {
	program := compileTool(t, `async function execute() {
		return toolResult({structuredContent: {id: "original"}, provider: {id: "provider"},
			content: [{type: "text", text: "original", annotations: {audience: ["assistant"]}}]});
	}`, nil)
	first := evaluateRichResult(t, program)

	// Mutating every returned representation cannot alter the reusable program or next VM.
	first.StructuredContent[0] = 'x'
	first.Provider[0] = 'x'
	*first.Content[0].Text = "changed"
	first.Content[0].Annotations.Audience[0] = "user"
	second := evaluateRichResult(t, program)
	require.JSONEq(t, `{"id":"original"}`, string(second.StructuredContent))
	require.JSONEq(t, `{"id":"provider"}`, string(second.Provider))
	require.Equal(t, "original", *second.Content[0].Text)
	require.Equal(t, []string{"assistant"}, second.Content[0].Annotations.Audience)
}

// TestToolResultConcurrentRuntimes keeps helper branding and authored state private to each invocation.
func TestToolResultConcurrentRuntimes(t *testing.T) {
	program := compileTool(t, `let calls = 0;
		async function execute() {
			return toolResult({structuredContent: {id: params.id, calls: ++calls}});
		}
	`, nil)
	type observation struct {
		id     int
		result *javascript.Result
		err    error
	}
	results := make(chan observation, 8)
	for id := range 8 {
		input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"id": id}})}
		// One immutable compiled program produces independently branded results in concurrent VMs.
		go func() {
			result, err := program.Evaluate(context.Background(), input)
			results <- observation{id: id, result: result, err: err}
		}()
	}
	for range 8 {
		observed := <-results
		require.NoError(t, observed.err)
		var values map[string]int
		require.NoError(t, json.Unmarshal(observed.result.StructuredContent, &values))
		require.Equal(t, map[string]int{"id": observed.id, "calls": 1}, values)
		require.NotNil(t, observed.result.Content)
		require.Empty(t, observed.result.Content)
	}
}

// TestToolResultWithHTTP translates fetched content through the same rich-result path.
func TestToolResultWithHTTP(t *testing.T) {
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader(`{"title":"Fetched report"}`))), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		const body = await (await fetch("https://provider.example/report")).json();
		return toolResult({structuredContent: body, content: [{type: "text", text: body.title}]});
	}`, nil)

	// Awaited HTTP data is preserved as both explicit rich content and structured output.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `{"title":"Fetched report"}`, string(result.StructuredContent))
	require.Len(t, result.Content, 1)
	require.Equal(t, "Fetched report", *result.Content[0].Text)
}
