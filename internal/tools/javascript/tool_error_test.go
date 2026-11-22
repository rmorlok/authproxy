package javascript_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// evaluateToolError checks that an explicit authored failure is detached domain
// data rather than a Go execution failure or a partially successful result.
func evaluateToolError(t *testing.T, program *javascript.ToolProgram) *api.ToolResultError {
	t.Helper()
	result, err := program.Evaluate(context.Background(), fetchInput(t))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Error)
	require.NotNil(t, result.Content)
	require.Empty(t, result.Content)
	require.Nil(t, result.StructuredContent)
	require.Nil(t, result.Provider)
	// Retry and side-effect policy remain with the trusted invocation caller.
	require.False(t, result.Error.Retryable)
	require.Empty(t, result.Error.ExecutionState)
	return result.Error
}

// TestToolErrorDetails preserves authored payloads, including omitted versus
// explicit null details, with either supported constructor calling style.
func TestToolErrorDetails(t *testing.T) {
	for _, test := range []struct {
		name, arguments, details string
	}{
		{"omitted", `"TEAM_LOOKUP_FAILED", "Could not resolve team"`, ""},
		{"undefined", `"TEAM_LOOKUP_FAILED", "Could not resolve team", undefined`, ""},
		{"null", `"TEAM_LOOKUP_FAILED", "Could not resolve team", null`, `null`},
		{"false", `"TEAM_LOOKUP_FAILED", "Could not resolve team", false`, `false`},
		{"zero", `"TEAM_LOOKUP_FAILED", "Could not resolve team", 0`, `0`},
		{"string", `"TEAM_LOOKUP_FAILED", "Could not resolve team", "safe detail"`, `"safe detail"`},
		{"array", `"TEAM_LOOKUP_FAILED", "Could not resolve team", [1,"two"]`, `[1,"two"]`},
		{"object", `"TEAM_LOOKUP_FAILED", "Could not resolve team", {upstreamStatus: 404}`, `{"upstreamStatus":404}`},
	} {
		for _, constructor := range []string{"new ToolError", "ToolError"} {
			t.Run(test.name+"/"+constructor, func(t *testing.T) {
				// Only an explicit root rejection exposes this intentionally public payload.
				program := compileTool(t, "async function execute() { throw "+constructor+"("+test.arguments+"); }", nil)
				result := evaluateToolError(t, program)
				require.Equal(t, "TEAM_LOOKUP_FAILED", result.Code)
				require.Equal(t, "Could not resolve team", result.Message)
				if test.details == "" {
					require.Nil(t, result.Details)
				} else {
					require.JSONEq(t, test.details, string(result.Details))
				}
			})
		}
	}
}

// TestToolErrorConstructorRejectsInvalidArguments uses catchable fixed failures
// for invalid arity or nonprimitive fields without authored coercion.
func TestToolErrorConstructorRejectsInvalidArguments(t *testing.T) {
	for name, arguments := range map[string]string{
		"missing arguments": "",
		"missing message":   `"FAILED"`,
		"extra argument":    `"FAILED", "Failed", {}, true`,
		"numeric code":      `42, "Failed"`,
		"boxed code":        `new String("FAILED"), "Failed"`,
		"null message":      `"FAILED", null`,
		"boxed message":     `"FAILED", new String("Failed")`,
		"coercion":          `{toString() { throw "private coercion"; }}, "Failed"`,
	} {
		t.Run(name, func(t *testing.T) {
			// Construction failures do not become authored public ToolError values.
			program := compileTool(t, "async function execute() { throw new ToolError("+arguments+"); }", nil)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}

	// Authors can recover ordinary constructor misuse without exhausting policy.
	program := compileTool(t, `async function execute() {
		try { ToolError(7, "wrong type"); } catch (_) { return "recovered"; }
	}`, nil)
	result := evaluateRichResult(t, program)
	require.Nil(t, result.Error)
	require.Equal(t, "recovered", *result.Content[0].Text)
}

// TestToolErrorRejectsInvalidPayloads validates all selected public fields while
// keeping failed translation atomic and its diagnostics opaque.
func TestToolErrorRejectsInvalidPayloads(t *testing.T) {
	for name, arguments := range map[string]string{
		"blank code":        `" \t", "Failed"`,
		"blank message":     `"FAILED", " \n"`,
		"reserved code":     `"OUTPUT_INVALID", "Failed"`,
		"reserved revision": `"TOOL_REVISION_CHANGED", "Failed"`,
		"nonfinite details": `"FAILED", "Failed", NaN`,
		"cyclic details":    `"FAILED", "Failed", (() => {const v = {}; v.self = v; return v;})()`,
		"getter":            `"FAILED", "Failed", {get private() { throw "private getter"; }}`,
		"proxy":             `"FAILED", "Failed", new Proxy({}, {ownKeys() { throw "private trap"; }})`,
		"toJSON":            `"FAILED", "Failed", {toJSON() { throw "private hook"; }}`,
		"nested helper":     `"FAILED", "Failed", {cause: new ToolError("NESTED", "Nested")}`,
		"lone code unit":    `"FAILED\ud800", "Failed"`,
		"lone message unit": `"FAILED", "Failed\udfff"`,
		"lone details unit": `"FAILED", "Failed", {value: "\ud800"}`,
		"lone details key":  `"FAILED", "Failed", {["\ud800"]: true}`,
	} {
		t.Run(name, func(t *testing.T) {
			// Branding alone does not bypass public shape, JSON, or string-fidelity checks.
			program := compileTool(t, "async function execute() { throw new ToolError("+arguments+"); }", nil)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrInvalidOutput)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}
}

// TestToolErrorRecognizesOnlyRejectedHandles prevents lookalikes, proxies, and
// success helpers from exporting ordinary private thrown values.
func TestToolErrorRecognizesOnlyRejectedHandles(t *testing.T) {
	for name, body := range map[string]string{
		"string":        `throw "private";`,
		"native error":  `throw new Error("private");`,
		"lookalike":     `throw {code: "FAILED", message: "private", details: {private: true}};`,
		"wrong helper":  `throw toolResult({structuredContent: "private"});`,
		"forged":        `throw Object.create(Object.getPrototypeOf(new ToolError("FAILED", "private")));`,
		"copied":        `throw Object.assign({}, new ToolError("FAILED", "private"));`,
		"proxy":         `throw new Proxy(new ToolError("FAILED", "private"), {});`,
		"detached root": `Promise.reject("private detached"); throw new ToolError("FAILED", "Safe");`,
	} {
		t.Run(name, func(t *testing.T) {
			// Unrecognized or unrelated failures retain the generic sanitized error category.
			program := compileTool(t, "async function execute() { "+body+" }", nil)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}

	for _, value := range []string{
		`new ToolError("FAILED", "Failed")`,
		`{error: new ToolError("FAILED", "Failed")}`,
		`toolResult({error: {code: "FAILED", message: "Failed"}})`,
	} {
		t.Run("returned/"+value, func(t *testing.T) {
			// A fulfilled promise cannot promote an error handle or supplied error field to a domain failure.
			program := compileTool(t, "async function execute() { return "+value+"; }", nil)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrInvalidOutput)
			require.Nil(t, result)
		})
	}
}

// TestToolErrorCaughtAndRethrown preserves normal JavaScript control flow while
// keeping a recognized handle's identity across an await boundary.
func TestToolErrorCaughtAndRethrown(t *testing.T) {
	program := compileTool(t, `async function execute() {
		const failure = new ToolError("FAILED", "Failed", {attempt: 1});
		try { throw failure; } catch (caught) {
			await Promise.resolve();
			throw caught;
		}
	}`, nil)
	require.JSONEq(t, `{"attempt":1}`, string(evaluateToolError(t, program).Details))

	// Catching an authored failure allows an ordinary successful fallback.
	program = compileTool(t, `async function execute() {
		try { throw new ToolError("FAILED", "Failed"); }
		catch (_) { return {fallback: true}; }
	}`, nil)
	result := evaluateRichResult(t, program)
	require.Nil(t, result.Error)
	require.JSONEq(t, `{"fallback":true}`, string(result.StructuredContent))
}

// TestToolErrorInitialization installs only a pure helper before authored code;
// initialization failures never become an invocation's authored error result.
func TestToolErrorInitialization(t *testing.T) {
	program := compileTool(t, `const failure = new ToolError("FAILED", "Failed");
		async function execute() { throw failure; }`, nil)

	// A publication-valid constant has no connection or network dependency.
	require.NoError(t, program.Validate(context.Background()))
	require.Equal(t, "FAILED", evaluateToolError(t, program).Code)

	for _, initialization := range []string{
		`throw new ToolError("FAILED", "private initialization");`,
		`Promise.reject(new ToolError("FAILED", "private initialization"));`,
	} {
		program = compileTool(t, initialization+` async function execute() { return true; }`, nil)
		// Both publication and invocation reject initialization before interpreting a root result.
		require.ErrorIs(t, program.Validate(context.Background()), apjsruntime.ErrEvaluation)
		result, err := program.Evaluate(context.Background(), fetchInput(t))
		require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
		require.Nil(t, result)
	}

	// Pure response transforms retain no Tool-specific error capability.
	expression := compileExpression(t, `typeof ToolError`)
	value := evaluateExpression(t, expression, javascript.Input{Context: expressionContext(t, render.Input{})})
	require.JSONEq(t, `"undefined"`, string(value))
}

// TestToolErrorCapturesIntrinsics keeps private recognition and the captured
// payload independent of globals that authored code later replaces.
func TestToolErrorCapturesIntrinsics(t *testing.T) {
	program := compileTool(t, `async function execute() {
		WeakMap.prototype.get = () => ({code: "FORGED", message: "Forged"});
		WeakMap.prototype.has = () => false;
		WeakMap.prototype.set = () => { throw "private"; };
		Object.create = () => { throw "private"; };
		Object.freeze = () => { throw "private"; };
		globalThis.Error = () => { throw "private"; };
		JSON.stringify = () => "private";
		throw new ToolError("FAILED", "Could not open \ud83d\ude00", {literal: "\\ud800"});
	}`, nil)

	// Safe Unicode and literal escape text survive the captured conversion path.
	result := evaluateToolError(t, program)
	require.Equal(t, "FAILED", result.Code)
	require.Equal(t, "Could not open 😀", result.Message)
	require.JSONEq(t, `{"literal":"\\ud800"}`, string(result.Details))
}

// TestToolErrorOwnership gives each invocation independent Go data even when
// one compiled program is reused concurrently and callers mutate prior results.
func TestToolErrorOwnership(t *testing.T) {
	program := compileTool(t, `let calls = 0;
		async function execute(params) {
			throw new ToolError("FAILED", "Failed", {id: params.id ?? null, calls: ++calls});
		}`, nil)
	type observation struct {
		id     int
		result *javascript.Result
		err    error
	}
	observed := make(chan observation, 8)
	for id := range 8 {
		input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"id": id}})}
		// Each VM creates its own brand registry, authored globals, and detached output.
		go func() {
			result, err := program.Evaluate(context.Background(), input)
			observed <- observation{id: id, result: result, err: err}
		}()
	}
	for range 8 {
		entry := <-observed
		require.NoError(t, entry.err)
		require.NotNil(t, entry.result)
		require.NotNil(t, entry.result.Error)
		var details map[string]int
		require.NoError(t, json.Unmarshal(entry.result.Error.Details, &details))
		require.Equal(t, map[string]int{"id": entry.id, "calls": 1}, details)
		entry.result.Error.Details[0] = 'x'
		entry.result.Error.Code = "changed"
	}

	// Caller-owned mutations never persist into the next invocation.
	failure := evaluateToolError(t, program)
	require.Equal(t, "FAILED", failure.Code)
	require.JSONEq(t, `{"id":null,"calls":1}`, string(failure.Details))
}

// TestToolErrorWithHTTP retains evidence independently of authored details and
// never uses an authored error as an instruction to retry upstream work.
func TestToolErrorWithHTTP(t *testing.T) {
	for _, afterSend := range []bool{false, true} {
		name, prefix := "before send", ""
		expectedCalls := int32(0)
		expectedState := api.ToolExecutionStateNotStarted
		if afterSend {
			name = "after send"
			prefix = `await context.connections[0].fetch("https://provider.example/action");`
			expectedCalls = 1
			expectedState = api.ToolExecutionStateMayHaveExecuted
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())
			program := compileTool(t, `async function execute() {
				`+prefix+`
				throw new ToolError("ACTION_FAILED", "Action failed", {retryable: true, executionState: "completed"});
			}`, nil)

			// Details may use arbitrary business keys but cannot set trusted outcome fields.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			require.NoError(t, err)
			require.NotNil(t, result.Error)
			require.JSONEq(t, `{"retryable":true,"executionState":"completed"}`, string(result.Error.Details))
			require.False(t, result.Error.Retryable)
			require.Empty(t, result.Error.ExecutionState)
			require.Equal(t, expectedState, options.Session.ExecutionState())
			require.Equal(t, expectedCalls, calls.Load())
		})
	}
}
