package httpcodec_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// transformResponseLimits permits ordinary projections while keeping the HTTP
// response, JavaScript runtime, and final envelope allowances independent.
func transformResponseLimits() httpcodec.ResponseLimits {
	limits := responseLimits()
	limits.Transform = javascript.Limits{
		MaxSourceBytes: 4096, MaxInputBytes: 32768, MaxOutputBytes: 16384,
		MaxDepth: 32, MaxNodes: 10000, MaxStackDepth: 128, Timeout: 2 * time.Second,
	}
	return limits
}

// transformResponseInput captures the same safe invocation data that request
// templates receive, independently from the provider's response bytes.
func transformResponseInput(t *testing.T) httpcodec.ResponseInput {
	t.Helper()
	input := responseInput()
	snapshot, err := render.NewContext(render.Input{
		Params:       map[string]any{"targetId": "contact-1"},
		Cfg:          map[string]any{"tenant": "acme"},
		Labels:       map[string]string{"team": "blue"},
		Annotations:  map[string]string{"note": "captured"},
		ConnectionID: input.Metadata.ConnectionID, ToolID: input.ToolID, Revision: input.Revision,
	}, render.Limits{MaxBytes: 32768, MaxDepth: 32, MaxSteps: 10000})
	require.NoError(t, err)
	input.Context = snapshot
	return input
}

// compileResponseTransform captures one expression through the public HTTP compiler.
func compileResponseTransform(t *testing.T, expression string, limits httpcodec.ResponseLimits) *httpcodec.Response {
	t.Helper()
	return compileResponse(t, &tools.HTTPResponse{TransformJavascript: &expression}, limits)
}

// requireTransformFailure checks that failed projections discard partial output
// and retain execution knowledge without exposing provider or configuration data.
func requireTransformFailure(t *testing.T, result *api.ToolResult, code string, state api.ToolExecutionState) {
	t.Helper()
	require.True(t, result.IsError)
	require.NotNil(t, result.Error)
	require.Equal(t, code, result.Error.Code)
	require.Equal(t, state, result.Error.ExecutionState)
	require.False(t, result.Error.Retryable)
	require.Nil(t, result.StructuredContent)
	require.Empty(t, result.Content)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
}

// TestResponseTransformContextAndHeaders exposes normalized response metadata
// alongside captured invocation data, with ContentType controlling decoding.
func TestResponseTransformContextAndHeaders(t *testing.T) {
	program := compileResponseTransform(t, `({
		id: params.targetId, tenant: context.cfg.tenant, label: labels.team,
		note: annotations.note, connection: context.connection.id,
		revision: context.tool.revision, aliases: cfg === context.cfg,
		status: response.status, mediaType: response.mediaType,
		headers: response.headers, body: response.body
	})`, transformResponseLimits())
	input := transformResponseInput(t)
	input.Body = []byte(`{"name":"Ada"}`)
	input.Headers = http.Header{
		"X-Request-Id": {"first", "second"}, "X-Empty": nil,
		"Content-Type": {"text/plain"}, "X-Tab": {"one\ttwo"},
	}

	// Header casing changes only the keys; repeated values and authoritative
	// MIME metadata survive without mutating the caller's header map.
	result := mapResponse(t, program, input)
	require.False(t, result.IsError)
	require.Empty(t, result.Content)
	require.JSONEq(t, `{
		"id":"contact-1","tenant":"acme","label":"blue","note":"captured",
		"connection":"cxn_01example0000001","revision":7,"aliases":true,
		"status":200,"mediaType":"application/json","body":{"name":"Ada"},
		"headers":{"x-request-id":["first","second"],"x-empty":[],"content-type":["text/plain"],"x-tab":["one\ttwo"]}
	}`, string(result.StructuredContent))
	require.Equal(t, []string{"first", "second"}, input.Headers["X-Request-Id"])
	require.NotContains(t, input.Headers, "x-request-id")
}

// TestResponseTransformBodyRepresentations supplies a single documented body
// representation for each MIME family, including explicit null versus no body.
func TestResponseTransformBodyRepresentations(t *testing.T) {
	binary := []byte{0, 255, 128}
	encodedBinary := `{"encoding":"base64","data":"` + base64.StdEncoding.EncodeToString(binary) + `"}`
	for _, test := range []struct {
		name, contentType, effectiveType string
		body                             []byte
		wantBody                         string
	}{
		{"JSON object", "application/json", "application/json", []byte(`{"id":"9007199254740993"}`), `{"id":"9007199254740993"}`},
		{"JSON null", "application/json", "application/json", []byte(`null`), `null`},
		{"JSON scalar", "application/problem+json", "application/problem+json", []byte(`false`), `false`},
		{"JSON string", "application/json", "application/json", []byte(`"Ada"`), `"Ada"`},
		{"empty JSON", "application/json", "application/json", nil, ""},
		{"UTF-8 text", "text/plain; charset=utf-8", "text/plain; charset=utf-8", []byte("Hello, 雪\n"), `"Hello, 雪\n"`},
		{"ASCII text", "text/plain; charset=us-ascii", "text/plain; charset=us-ascii", []byte("Hello"), `"Hello"`},
		{"unsupported text charset", "text/plain; charset=windows-1252", "text/plain; charset=windows-1252", binary, encodedBinary},
		{"image", "image/png", "image/png", binary, encodedBinary},
		{"audio", "audio/wav", "audio/wav", binary, encodedBinary},
		{"document", "application/pdf", "application/pdf", binary, encodedBinary},
		{"missing MIME", "", "application/octet-stream", binary, encodedBinary},
		{"invalid optional parameters", "application/json; broken", "application/json", []byte(`{"ok":true}`), `{"ok":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Returning the prepared response reveals its exact ordinary JSON shape.
			program := compileResponseTransform(t, `response`, transformResponseLimits())
			input := transformResponseInput(t)
			input.ContentType, input.Body = test.contentType, test.body
			result := mapResponse(t, program, input)
			require.False(t, result.IsError)
			want := `{"status":200,"headers":{},"mediaType":"` + test.effectiveType + `"`
			if test.wantBody != "" {
				want += `,"body":` + test.wantBody
			}
			require.JSONEq(t, want+`}`, string(result.StructuredContent))
		})
	}
}

// TestResponseTransformResultKinds preserves transformed JSON values and adds
// a text block only for a top-level string, without interpreting rich-result fields.
func TestResponseTransformResultKinds(t *testing.T) {
	for _, test := range []struct {
		name, expression, want string
		text                   *string
	}{
		{"object", `({name: "Ada"})`, `{"name":"Ada"}`, nil},
		{"array", `[1, false, null]`, `[1,false,null]`, nil},
		{"null", `null`, `null`, nil},
		{"boolean", `false`, `false`, nil},
		{"number", `42`, `42`, nil},
		{"string", `"Hello, 雪\n"`, `"Hello, 雪\n"`, util.ToPtr("Hello, 雪\n")},
		{"empty string", `""`, `""`, util.ToPtr("")},
		{"surrogate pair", `"\ud83d\ude00"`, `"😀"`, util.ToPtr("😀")},
		{"rich-result-shaped data", `({isError: true, content: [{type:"text", text:"ordinary data"}]})`, `{"isError":true,"content":[{"type":"text","text":"ordinary data"}]}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := transformResponseInput(t)
			result := mapResponse(t, compileResponseTransform(t, test.expression, transformResponseLimits()), input)
			require.False(t, result.IsError)
			require.JSONEq(t, test.want, string(result.StructuredContent))
			if test.text == nil {
				require.Empty(t, result.Content)
			} else {
				require.Len(t, result.Content, 1)
				require.Equal(t, api.ToolContentTypeText, result.Content[0].Type)
				require.Equal(t, test.text, result.Content[0].Text)
			}
		})
	}
}

// TestResponseTransformStatusPrecedence prevents unused transform inputs from
// replacing status errors, including explicit rules targeting a successful status.
func TestResponseTransformStatusPrecedence(t *testing.T) {
	limits := transformResponseLimits()
	limits.MaxResponseBytes, limits.MaxOutputBytes, limits.Transform.MaxInputBytes = 16, 16, 16
	program := compileResponse(t, &tools.HTTPResponse{
		TransformJavascript: util.ToPtr(`(() => { throw new Error("private-transform"); })()`),
		Errors:              []tools.HTTPErrorRule{{Statuses: []int{200, 404}, Code: "KNOWN_STATUS", Message: "A known status occurred."}},
	}, limits)
	for _, test := range []struct {
		status int
		code   string
	}{
		{200, "KNOWN_STATUS"}, {404, "KNOWN_STATUS"}, {500, "UPSTREAM_FAILED"}, {99, "UPSTREAM_PROTOCOL_ERROR"},
	} {
		// Nil context, invalid headers/MIME, and oversized bytes all remain unused.
		input := responseInput()
		input.StatusCode = test.status
		input.ContentType = "private-invalid-type"
		input.Headers = http.Header{"private invalid header": {"private-value\r\n"}}
		input.Body = []byte(strings.Repeat("private-", 100))
		input.ExecutionState = api.ToolExecutionStateMayHaveExecuted
		result := mapResponse(t, program, input)
		requireTransformFailure(t, result, test.code, input.ExecutionState)
	}
}

// TestResponseTransformInvalidInput fails safely before author code receives
// malformed response metadata, JSON, text, or an invalid captured snapshot.
func TestResponseTransformInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name, code string
		change     func(*httpcodec.ResponseInput)
	}{
		{"invalid MIME", "UPSTREAM_PROTOCOL_ERROR", func(i *httpcodec.ResponseInput) { i.ContentType = "private-invalid-type" }},
		{"malformed JSON", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) { i.Body = []byte(`{"private-field":`) }},
		{"non-finite JSON", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) { i.Body = []byte(`{"private-field":1e400}`) }},
		{"unpaired JSON surrogate", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) { i.Body = []byte(`{"private-field":"\ud800"}`) }},
		{"invalid text", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) { i.ContentType = "text/plain"; i.Body = []byte{0xff} }},
		{"non-ASCII text", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) {
			i.ContentType = "text/plain; charset=us-ascii"
			i.Body = []byte("雪")
		}},
		{"zero context", "OUTPUT_INVALID", func(i *httpcodec.ResponseInput) { i.Context = new(render.Context) }},
		{"duplicate header", "UPSTREAM_PROTOCOL_ERROR", func(i *httpcodec.ResponseInput) { i.Headers = http.Header{"X-Private": {"one"}, "x-private": {"two"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := transformResponseInput(t)
			test.change(&input)
			input.ExecutionState = api.ToolExecutionStateMayHaveExecuted
			program := compileResponseTransform(t, `response.body`, transformResponseLimits())
			requireTransformFailure(t, mapResponse(t, program, input), test.code, input.ExecutionState)
		})
	}

	// A missing snapshot is a caller error, not a fabricated transform input.
	input := responseInput()
	program := compileResponseTransform(t, `42`, transformResponseLimits())
	result, err := program.Map(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, result)
}

// TestResponseTransformOmitsInvalidHeaders drops malformed provider metadata
// instead of failing an otherwise successful response.
func TestResponseTransformOmitsInvalidHeaders(t *testing.T) {
	for _, test := range []struct {
		name    string
		headers http.Header
	}{
		{"header token", http.Header{"private invalid": {"value"}}},
		{"header name UTF-8", http.Header{string([]byte{0xff}): {"value"}}},
		{"header value UTF-8", http.Header{"X-Private": {string([]byte{0xff})}}},
		{"obs-text filename", http.Header{"Content-Disposition": {"attachment; filename=\"r\xe9sum\xe9.pdf\""}}},
		{"header line break", http.Header{"X-Private": {"private-value\r\nInjected: true"}}},
		{"header NUL", http.Header{"X-Private": {"private-value\x00"}}},
		{"one bad value among good", http.Header{"X-Private": {"ok", "private-value\x00"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			program := compileResponseTransform(t, `({headers: response.headers, body: response.body})`, transformResponseLimits())
			input := transformResponseInput(t)
			input.Body = []byte(`{"name":"Ada"}`)
			input.Headers = test.headers.Clone()
			input.Headers["X-Request-Id"] = []string{"req-7"}

			// The response still succeeds; only the invalid header disappears.
			result := mapResponse(t, program, input)
			require.False(t, result.IsError)
			require.JSONEq(t, `{"headers":{"x-request-id":["req-7"]},"body":{"name":"Ada"}}`, string(result.StructuredContent))
			require.NotContains(t, string(result.StructuredContent), "private-")
		})
	}
}

// TestResponseTransformFailuresArePrivate classifies author failures without
// leaking thrown values, code, provider data, or partially constructed content.
func TestResponseTransformFailuresArePrivate(t *testing.T) {
	for _, expression := range []string{
		`(() => { throw new Error(response.body.privateValue); })()`,
		`undefined`, `({answer: undefined})`, `NaN`, `Promise.resolve(42)`,
		`({ get answer() { throw new Error("private-getter"); } })`,
		`(() => { const value = {}; value.self = value; return value; })()`,
		`"\ud800"`, `"\udfff"`,
	} {
		t.Run(expression, func(t *testing.T) {
			input := transformResponseInput(t)
			input.Body = []byte(`{"privateValue":"private-provider-data"}`)
			program := compileResponseTransform(t, expression, transformResponseLimits())
			requireTransformFailure(t, mapResponse(t, program, input), "OUTPUT_INVALID", input.ExecutionState)
		})
	}
}

// TestResponseTransformOutputLimits permits useful shrinking projections and
// distinguishes HTTP content allowances from independent JavaScript limits.
func TestResponseTransformOutputLimits(t *testing.T) {
	limits := transformResponseLimits()
	limits.MaxOutputBytes = len(`{"id":"contact-1"}`)
	input := transformResponseInput(t)
	input.Body = []byte(`{"id":"contact-1","unused":"` + strings.Repeat("x", 1024) + `"}`)
	program := compileResponseTransform(t, `({id: response.body.id})`, limits)

	// The response exceeds the output cap, but its smaller projection fits exactly.
	result := mapResponse(t, program, input)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"id":"contact-1"}`, string(result.StructuredContent))
	limits.MaxOutputBytes--
	program = compileResponseTransform(t, `({id: response.body.id})`, limits)
	requireTransformFailure(t, mapResponse(t, program, input), "OUTPUT_TOO_LARGE", input.ExecutionState)

	// Strings charge encoded JSON bytes, including quotes, before adding text.
	limits = transformResponseLimits()
	limits.MaxOutputBytes = len(`"x"`)
	program = compileResponseTransform(t, `"x"`, limits)
	require.False(t, mapResponse(t, program, input).IsError)
	limits.MaxOutputBytes--
	program = compileResponseTransform(t, `"x"`, limits)
	requireTransformFailure(t, mapResponse(t, program, input), "OUTPUT_TOO_LARGE", input.ExecutionState)

	// A tighter runtime output allowance is an execution-policy failure instead.
	limits = transformResponseLimits()
	limits.Transform.MaxOutputBytes = 2
	program = compileResponseTransform(t, `"x"`, limits)
	requireTransformFailure(t, mapResponse(t, program, input), "EXECUTION_LIMIT_EXCEEDED", input.ExecutionState)

	// A transform cannot bypass the decoded-response cap by ignoring its body.
	limits = transformResponseLimits()
	limits.MaxResponseBytes = len(input.Body) - 1
	program = compileResponseTransform(t, `42`, limits)
	requireTransformFailure(t, mapResponse(t, program, input), "OUTPUT_TOO_LARGE", input.ExecutionState)

	// Final envelope accounting includes both representations of a transformed
	// string, even when its JSON output fits the earlier content allowances.
	limits = transformResponseLimits()
	expression := `"` + strings.Repeat("x", 512) + `"`
	program = compileResponseTransform(t, expression, limits)
	result = mapResponse(t, program, input)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	limits.MaxEncodedResultBytes = len(encoded) - 1
	program = compileResponseTransform(t, expression, limits)
	requireTransformFailure(t, mapResponse(t, program, input), "OUTPUT_TOO_LARGE", input.ExecutionState)
}

// TestResponseTransformRuntimeLimits maps input, structural, and stack guards
// to execution-limit errors while preserving the executor's recorded state.
func TestResponseTransformRuntimeLimits(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		change           func(*javascript.Limits)
	}{
		{"input", `42`, func(l *javascript.Limits) { l.MaxInputBytes = 16 }},
		{"output nodes", `new Array(1000).fill(0)`, func(l *javascript.Limits) { l.MaxNodes = 128 }},
		{"output depth", `[[[[[[[1]]]]]]]`, func(l *javascript.Limits) { l.MaxDepth = 5 }},
		{"stack", `(() => { function recurse() { return recurse(); } return recurse(); })()`, func(l *javascript.Limits) { l.MaxStackDepth = 32 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := transformResponseLimits()
			test.change(&limits.Transform)
			program := compileResponseTransform(t, test.expression, limits)
			input := transformResponseInput(t)
			input.ExecutionState = api.ToolExecutionStateMayHaveExecuted
			requireTransformFailure(t, mapResponse(t, program, input), "EXECUTION_LIMIT_EXCEEDED", input.ExecutionState)
		})
	}
}

// mapTransformWithWatchdog bounds tests of interruption even if a future
// integration regression stops propagating cancellation into JavaScript.
func mapTransformWithWatchdog(t *testing.T, ctx context.Context, cancel context.CancelFunc, program *httpcodec.Response, input httpcodec.ResponseInput) (*api.ToolResult, error) {
	t.Helper()
	type outcome struct {
		result *api.ToolResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() { result, err := program.Map(ctx, input); done <- outcome{result, err} }()
	t.Cleanup(cancel)
	select {
	case value := <-done:
		return value.result, value.err
	case <-time.After(5 * time.Second):
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("response transform did not stop before watchdog expired")
		return nil, nil
	}
}

// TestResponseTransformTimeoutAndCancellation distinguishes configured policy
// timeouts from caller deadlines and keeps the compiled program reusable.
func TestResponseTransformTimeoutAndCancellation(t *testing.T) {
	limits := transformResponseLimits()
	limits.Transform.Timeout = 100 * time.Millisecond
	program := compileResponseTransform(t, `(() => { if (response.body.busy) { while (true) {} } return 42; })()`, limits)
	input := transformResponseInput(t)
	input.Body = []byte(`{"busy":true}`)
	ctx, cancel := context.WithCancel(context.Background())

	// Runtime time limits produce safe Tool errors while the caller remains live.
	result, err := mapTransformWithWatchdog(t, ctx, cancel, program, input)
	require.NoError(t, err)
	require.NoError(t, ctx.Err())
	requireTransformFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", input.ExecutionState)

	// An earlier caller deadline remains an ordinary Go context error.
	limits.Transform.Timeout = 2 * time.Second
	program = compileResponseTransform(t, `(() => { if (response.body.busy) { while (true) {} } return 42; })()`, limits)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	result, err = mapTransformWithWatchdog(t, ctx, cancel, program, input)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	result, err = mapTransformWithWatchdog(t, ctx, cancel, program, input)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	// An interrupted invocation cannot leave state in a later transformation.
	input.Body = []byte(`{"busy":false}`)
	result = mapResponse(t, program, input)
	require.False(t, result.IsError)
	require.JSONEq(t, `42`, string(result.StructuredContent))
}

// TestResponseTransformCompilation checks source accounting and rejects invalid
// expressions without executing source or exposing its private content.
func TestResponseTransformCompilation(t *testing.T) {
	for _, expression := range []string{"", " ", `({privateMarker:`, `1; 2`, string([]byte{0xff})} {
		limits := transformResponseLimits()
		program, err := httpcodec.CompileResponse(&tools.HTTPResponse{TransformJavascript: &expression}, limits)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "privateMarker")
		require.Nil(t, program)
	}

	// Rules and expression bytes share the HTTP source allowance; the runtime
	// also applies its own independently configured source allowance.
	expression := `response.body`
	source := &tools.HTTPResponse{TransformJavascript: &expression, Errors: []tools.HTTPErrorRule{
		{Statuses: []int{404}, Code: "MISSING", Message: "Missing."},
	}}
	limits := transformResponseLimits()
	limits.MaxSourceBytes = len(expression) + 1 + len("404") + len("MISSING") + len("Missing.")
	limits.Transform.MaxSourceBytes = len(expression)
	compileResponse(t, source, limits)
	limits.MaxSourceBytes--
	_, err := httpcodec.CompileResponse(source, limits)
	require.Error(t, err)
	limits.MaxSourceBytes++
	limits.Transform.MaxSourceBytes--
	_, err = httpcodec.CompileResponse(source, limits)
	require.Error(t, err)

	// Existing passthrough callers do not need a JavaScript policy or context.
	program := compileResponse(t, nil, responseLimits())
	require.False(t, mapResponse(t, program, responseInput()).IsError)
	_, err = httpcodec.CompileResponse(source, responseLimits())
	require.Error(t, err)
}

// TestResponseTransformCopiesAndConcurrency shares one immutable program and
// invocation snapshot while every expression receives private native objects.
func TestResponseTransformCopiesAndConcurrency(t *testing.T) {
	expression := `(() => {
		const before = {name: response.body.name, header: response.headers["x-name"][0], tenant: cfg.tenant};
		response.body.name = "changed"; response.headers["x-name"][0] = "changed"; cfg.tenant = "changed";
		return before;
	})()`
	source := &tools.HTTPResponse{TransformJavascript: &expression}
	program := compileResponse(t, source, transformResponseLimits())
	input := transformResponseInput(t)
	input.Body = []byte(`{"name":"Ada"}`)
	input.Headers = http.Header{"X-Name": {"one"}}

	// Editing the authored source after compilation cannot change the plan.
	expression = `(() => { throw new Error("private-edited-source"); })()`
	want := `{"name":"Ada","header":"one","tenant":"acme"}`
	result := mapResponse(t, program, input)
	require.JSONEq(t, want, string(result.StructuredContent))
	result.StructuredContent[0] = 'x'
	require.JSONEq(t, want, string(mapResponse(t, program, input).StructuredContent))
	require.Equal(t, `{"name":"Ada"}`, string(input.Body))
	require.Equal(t, []string{"one"}, input.Headers["X-Name"])

	// Concurrent calls retain their original inputs and own their result bytes.
	type outcome struct {
		result *api.ToolResult
		err    error
	}
	results := make(chan outcome, 16)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() { result, err := program.Map(context.Background(), input); results <- outcome{result, err} })
	}
	workers.Wait()
	close(results)
	for value := range results {
		require.NoError(t, value.err)
		require.False(t, value.result.IsError)
		require.JSONEq(t, want, string(value.result.StructuredContent))
	}
}

// TestResponseTransformHasNoHostCapabilities confirms HTTP integration does
// not inject networking, timers, modules, or higher-level ToolResult helpers.
func TestResponseTransformHasNoHostCapabilities(t *testing.T) {
	program := compileResponseTransform(t, `({
		fetch: typeof fetch, require: typeof require, process: typeof process,
		setTimeout: typeof setTimeout, console: typeof console,
		ToolError: typeof ToolError, toolResult: typeof toolResult
	})`, transformResponseLimits())
	result := mapResponse(t, program, transformResponseInput(t))
	require.False(t, result.IsError)
	require.JSONEq(t, `{"fetch":"undefined","require":"undefined","process":"undefined","setTimeout":"undefined","console":"undefined","ToolError":"undefined","toolResult":"undefined"}`, string(result.StructuredContent))
}
