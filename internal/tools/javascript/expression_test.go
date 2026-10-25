package javascript_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// expressionLimits gives normal examples ample room while keeping every limit explicit.
func expressionLimits() javascript.Limits {
	return javascript.Limits{
		MaxSourceBytes: 64 << 10,
		MaxInputBytes:  64 << 10,
		MaxOutputBytes: 64 << 10,
		MaxDepth:       32,
		MaxNodes:       10000,
		MaxStackDepth:  128,
		Timeout:        2 * time.Second,
	}
}

// expressionContext captures ordinary input through the same immutable renderer snapshot as HTTP templates.
func expressionContext(t *testing.T, input render.Input) *render.Context {
	t.Helper()
	snapshot, err := render.NewContext(input, render.Limits{MaxBytes: 1 << 20, MaxDepth: 64, MaxSteps: 10000})
	require.NoError(t, err)
	return snapshot
}

// compileExpression prepares authored source without evaluating it.
func compileExpression(t *testing.T, source string) *javascript.Expression {
	t.Helper()
	compiled, err := javascript.CompileExpression(source, expressionLimits())
	require.NoError(t, err)
	return compiled
}

// evaluateExpression invokes a compiled expression and checks the result is ordinary JSON.
func evaluateExpression(t *testing.T, compiled *javascript.Expression, input javascript.Input) common.RawJSON {
	t.Helper()
	result, err := compiled.Evaluate(context.Background(), input)
	require.NoError(t, err)
	require.True(t, json.Valid(result), "result must be valid JSON")
	return result
}

func TestExpressionInvocationContext(t *testing.T) {
	// Capture configuration, user input, and provenance independently from the response.
	generation, catalog := uint64(7), uint64(11)
	input := javascript.Input{
		Context: expressionContext(t, render.Input{
			Params:              map[string]any{"name": "Ada", "count": 3},
			Cfg:                 map[string]any{"tenant": "acme", "nested": map[string]any{"region": "us"}},
			Labels:              map[string]string{"team": "blue"},
			Annotations:         map[string]string{"note": "captured"},
			ConnectionID:        "cxn_example",
			ConnectionNamespace: "root.product.owner",
			ToolID:              "tol_example",
			Revision:            9,
			ToolSetID:           "tls_example",
			ToolSetGeneration:   &generation,
			CatalogRevision:     &catalog,
		}),
		Response: common.RawJSON(`{"status":200,"body":{"id":"contact-1"}}`),
	}
	compiled := compileExpression(t, `({
		greeting: params.name + "@" + cfg.tenant,
		count: params.count + 2,
		connection: context.connection,
		tool: context.tool,
		region: context.cfg.nested.region,
		label: labels.team,
		annotation: annotations.note,
		response: response,
		aliases: cfg === context.cfg && labels === context.labels && annotations === context.annotations,
		paramsSeparate: !("params" in context),
		nativeObjects: Object.getPrototypeOf(params) === Object.prototype &&
			Object.getPrototypeOf(cfg) === Object.prototype
	})`)

	// Native JavaScript sees the documented aliases and only the allowed metadata.
	require.JSONEq(t, `{
		"greeting":"Ada@acme", "count":5,
		"connection":{"id":"cxn_example","namespace":"root.product.owner"},
		"tool":{"id":"tol_example","revision":9,"toolSetId":"tls_example","toolSetGeneration":7,"catalogRevision":11},
		"region":"us", "label":"blue", "annotation":"captured",
		"response":{"status":200,"body":{"id":"contact-1"}}, "aliases":true,"paramsSeparate":true,"nativeObjects":true
	}`, string(evaluateExpression(t, compiled, input)))
}

func TestExpressionOptionalResponseAndJSONValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{"missing response is empty", `response`, `{}`},
		{"object", `({name: "Ada", enabled: true, empty: null})`, `{"name":"Ada","enabled":true,"empty":null}`},
		{"array", `[1, "two", false, null]`, `[1,"two",false,null]`},
		{"string", `"line\n雪"`, `"line\n雪"`},
		{"number", `6 * 7`, `42`},
		{"boolean", `false`, `false`},
		{"null", `null`, `null`},
		{"standard number precision", `9007199254740993`, `9007199254740992`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Expression results retain JSON value kinds, with JavaScript number semantics.
			compiled := compileExpression(t, test.source)
			input := javascript.Input{Context: expressionContext(t, render.Input{})}
			require.JSONEq(t, test.want, string(evaluateExpression(t, compiled, input)))
		})
	}
}

func TestExpressionCompileValidation(t *testing.T) {
	for _, source := range []string{
		"", " ", "({", "const value = 1; value", "1; 2", "return 1",
		`1); (() => { throw new Error("private"); })(); (2`,
		string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(source, func(t *testing.T) {
			// Invalid syntax and multiple statements must never become an executable plan.
			compiled, err := javascript.CompileExpression(source, expressionLimits())
			require.ErrorIs(t, err, javascript.ErrInvalidSource)
			require.Nil(t, compiled)
		})
	}

	// Compiling valid but throwing code must not execute any authored JavaScript.
	compiled := compileExpression(t, `(() => { throw new Error("private-source-marker"); })()`)
	result, err := compiled.Evaluate(context.Background(), javascript.Input{Context: expressionContext(t, render.Input{})})
	require.ErrorIs(t, err, javascript.ErrEvaluation)
	require.NotContains(t, err.Error(), "private-source-marker")
	require.Nil(t, result)
}

func TestExpressionLimitsMustBePositive(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*javascript.Limits)
	}{
		{"source bytes", func(l *javascript.Limits) { l.MaxSourceBytes = 0 }},
		{"input bytes", func(l *javascript.Limits) { l.MaxInputBytes = 0 }},
		{"output bytes", func(l *javascript.Limits) { l.MaxOutputBytes = 0 }},
		{"depth", func(l *javascript.Limits) { l.MaxDepth = 0 }},
		{"nodes", func(l *javascript.Limits) { l.MaxNodes = 0 }},
		{"stack depth", func(l *javascript.Limits) { l.MaxStackDepth = 0 }},
		{"timeout", func(l *javascript.Limits) { l.Timeout = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A missing policy value cannot accidentally disable an execution guard.
			limits := expressionLimits()
			test.change(&limits)
			compiled, err := javascript.CompileExpression(`1`, limits)
			require.Error(t, err)
			require.Nil(t, compiled)
		})
	}

	// Source accounting is byte-based and accepts the exact configured boundary.
	source := `"雪"`
	limits := expressionLimits()
	limits.MaxSourceBytes = len(source)
	_, err := javascript.CompileExpression(source, limits)
	require.NoError(t, err)
	limits.MaxSourceBytes--
	compiled, err := javascript.CompileExpression(source, limits)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, compiled)
}

func TestExpressionErrorsDoNotExposeInvocationData(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{
		Cfg: map[string]any{"secret": "private-connection-value"},
	})}
	for _, source := range []string{
		`(() => { throw new Error(cfg.secret); })()`,
		`(() => { throw cfg.secret; })()`,
		`(() => { throw {toString() { return cfg.secret; }}; })()`,
		`(() => { unintendedGlobal = cfg.secret; return unintendedGlobal; })()`,
	} {
		t.Run(source, func(t *testing.T) {
			// Exception text and thrown objects are never returned to callers.
			result, err := compileExpression(t, source).Evaluate(context.Background(), input)
			require.ErrorIs(t, err, javascript.ErrEvaluation)
			require.NotContains(t, err.Error(), "private-connection-value")
			require.NotContains(t, err.Error(), source)
			require.Nil(t, result)
		})
	}
}

func TestExpressionHostFeaturesAreAbsent(t *testing.T) {
	// Pure expressions have ordinary language intrinsics but no injected host capability.
	compiled := compileExpression(t, `({
		fetch: typeof fetch, require: typeof require, process: typeof process,
		Deno: typeof Deno, Bun: typeof Bun, console: typeof console,
		setTimeout: typeof setTimeout, XMLHttpRequest: typeof XMLHttpRequest,
		WebSocket: typeof WebSocket, Buffer: typeof Buffer,
		math: Math.max(3, 5), json: JSON.parse('{"answer":42}').answer
	})`)
	result := evaluateExpression(t, compiled, javascript.Input{Context: expressionContext(t, render.Input{})})
	require.JSONEq(t, `{
		"fetch":"undefined","require":"undefined","process":"undefined",
		"Deno":"undefined","Bun":"undefined","console":"undefined",
		"setTimeout":"undefined","XMLHttpRequest":"undefined",
		"WebSocket":"undefined","Buffer":"undefined","math":5,"json":42
	}`, string(result))
}

func TestExpressionRejectsNonJSONResults(t *testing.T) {
	for _, source := range []string{
		`undefined`, `(() => {})`, `NaN`, `Infinity`, `-Infinity`,
		`1n`, `Symbol("value")`, `({[Symbol("key")]: 1})`,
		`({value: undefined})`, `[undefined]`, `({value() { return 1; }})`,
		`(() => { const value = {}; value.self = value; return value; })()`,
		`new Date(0)`, `/example/`, `new Map([["value", 1]])`, `new Set([1])`,
		`new Uint8Array([1, 2])`, `Promise.resolve(1)`, `(async () => 1)()`,
		`({ get answer() { throw new Error("private-getter-value"); } })`,
		`({ get answer() { while (true) {} } })`,
		`new Proxy({}, {get() { throw new Error("private-proxy-value"); }})`,
	} {
		t.Run(source, func(t *testing.T) {
			// Returning a non-JSON value must fail instead of silently dropping or coercing it.
			input := javascript.Input{Context: expressionContext(t, render.Input{})}
			ctx, cancel := context.WithCancel(context.Background())
			result, err := evaluateWithWatchdog(t, ctx, cancel, compileExpression(t, source), input)
			require.ErrorIs(t, err, javascript.ErrInvalidOutput)
			require.NotContains(t, err.Error(), "private-")
			require.Nil(t, result)
		})
	}
}

func TestExpressionFreshNativeCopiesAndReadonlyGlobals(t *testing.T) {
	// Preserve a single captured snapshot across mutations performed by several evaluations.
	params := map[string]any{"items": []any{map[string]any{"name": "original"}}}
	cfg := map[string]any{"nested": map[string]any{"tenant": "acme"}}
	response := common.RawJSON(`{"body":{"name":"upstream"}}`)
	input := javascript.Input{Context: expressionContext(t, render.Input{
		Params: params, Cfg: cfg, Labels: map[string]string{"team": "blue"},
		Annotations: map[string]string{"note": "captured"},
	}), Response: response}
	compiled := compileExpression(t, `(() => {
		const before = [params.items[0].name, cfg.nested.tenant, labels.team,
			annotations.note, response.body.name, typeof globalThis.previousRun,
			typeof Object.prototype.fromPreviousRun];
		params.items[0].name = "changed";
		context.cfg.nested.tenant = "changed";
		context.labels.team = "changed";
		context.annotations.note = "changed";
		response.body.name = "changed";
		globalThis.previousRun = true;
		Object.prototype.fromPreviousRun = true;
		return before;
	})()`)

	// Every run receives detached native objects even when the same plan and input are reused.
	for range 2 {
		require.JSONEq(t, `["original","acme","blue","captured","upstream","undefined","undefined"]`,
			string(evaluateExpression(t, compiled, input)))
	}
	require.Equal(t, "original", params["items"].([]any)[0].(map[string]any)["name"])
	require.Equal(t, "acme", cfg["nested"].(map[string]any)["tenant"])
	require.Equal(t, `{"body":{"name":"upstream"}}`, string(response))

	// Root bindings are fixed, so authored code cannot replace an invocation scope.
	for _, name := range []string{"params", "context", "response", "cfg", "labels", "annotations"} {
		t.Run(name, func(t *testing.T) {
			result, err := compileExpression(t, `(() => { `+name+` = {}; return true; })()`).Evaluate(context.Background(), input)
			require.ErrorIs(t, err, javascript.ErrEvaluation)
			require.Nil(t, result)
		})
	}
}

func TestExpressionOutputIgnoresModifiedIntrinsics(t *testing.T) {
	// Authored mutations to standard objects must not replace the host's JSON traversal.
	compiled := compileExpression(t, `(() => {
		Object.keys = () => [];
		Object.getOwnPropertyDescriptor = () => ({value: "forged"});
		Object.getOwnPropertyDescriptors = () => ({});
		Array.isArray = () => false;
		JSON.stringify = () => '"forged"';
		return {actual: [42, {name: "Ada"}]};
	})()`)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	require.JSONEq(t, `{"actual":[42,{"name":"Ada"}]}`, string(evaluateExpression(t, compiled, input)))
}

func TestExpressionConcurrentEvaluations(t *testing.T) {
	// One immutable plan and snapshot may serve concurrent callers with independent VMs.
	input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"count": 41}})}
	compiled := compileExpression(t, `(() => { params.count++; return params.count; })()`)
	const count = 24
	results := make(chan error, count)
	var workers sync.WaitGroup
	for range count {
		workers.Go(func() {
			result, err := compiled.Evaluate(context.Background(), input)
			if err == nil && string(result) != "42" {
				err = errors.New("evaluation observed another call's mutation")
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)

	// All calls observe the original captured value, including under the race detector.
	for err := range results {
		require.NoError(t, err)
	}
}

// evaluateWithWatchdog prevents a broken interruption path from blocking the entire test suite.
func evaluateWithWatchdog(t *testing.T, ctx context.Context, cancel context.CancelFunc, compiled *javascript.Expression, input javascript.Input) (common.RawJSON, error) {
	t.Helper()
	type outcome struct {
		result common.RawJSON
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := compiled.Evaluate(ctx, input)
		done <- outcome{result: result, err: err}
	}()
	t.Cleanup(cancel)

	select {
	case result := <-done:
		return result.result, result.err
	case <-time.After(5 * time.Second):
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("JavaScript evaluation failed to stop before the watchdog expired")
		return nil, nil
	}
}

func TestExpressionCancellationAndReuse(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"busy": true}})}
	compiled := compileExpression(t, `(() => { if (params.busy) { while (true) {} } return 42; })()`)

	// Already canceled invocations fail before any authored work can complete.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := evaluateWithWatchdog(t, ctx, cancel, compiled, input)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, result)

	// An already-expired caller deadline retains its classification at admission.
	ctx, cancel = context.WithDeadline(context.Background(), time.Time{})
	result, err = evaluateWithWatchdog(t, ctx, cancel, compiled, input)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, result)

	// A caller deadline interrupts a running loop; it is not converted to a JavaScript exception.
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	result, err = evaluateWithWatchdog(t, ctx, cancel, compiled, input)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, result)

	// Cancellation belongs to that invocation, so the same plan remains usable afterward.
	input.Context = expressionContext(t, render.Input{Params: map[string]any{"busy": false}})
	require.JSONEq(t, `42`, string(evaluateExpression(t, compiled, input)))
}

func TestExpressionConcurrentCallerCancellation(t *testing.T) {
	// Race short evaluations against caller cancellation. The policy allowance
	// is deliberately generous, so a policy-limit error would be misclassified.
	limits := expressionLimits()
	limits.Timeout = time.Minute
	compiled, err := javascript.CompileExpression(`params.count + 1`, limits)
	require.NoError(t, err)
	input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"count": 41}})}
	const count = 64
	start := make(chan struct{})
	results := make(chan error, count)
	cancels := make([]context.CancelFunc, 0, count)
	var workers sync.WaitGroup
	for range count {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		workers.Go(func() {
			<-start
			result, err := compiled.Evaluate(ctx, input)
			switch {
			case err == nil && string(result) != "42":
				err = errors.New("successful evaluation returned an unexpected result")
			case errors.Is(err, context.Canceled) && result != nil:
				err = errors.New("canceled evaluation returned partial output")
			case errors.Is(err, context.Canceled):
				err = nil
			}
			results <- err
		})
		workers.Go(func() { <-start; cancel() })
	}
	cancelAll := func() {
		for _, cancel := range cancels {
			cancel()
		}
	}
	t.Cleanup(cancelAll)
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	close(start)

	// Every worker must stop promptly, even if cancellation occurs at admission
	// or just as evaluation finishes. The watchdog cleans up before failing.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancelAll()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("concurrent canceled evaluations did not stop")
	}
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	// Cancellation of concurrent calls leaves the shared compiled plan usable.
	require.JSONEq(t, `42`, string(evaluateExpression(t, compiled, input)))
}

func TestExpressionTimeout(t *testing.T) {
	// The runtime's execution policy remains distinct from the caller's lifetime.
	limits := expressionLimits()
	limits.Timeout = 100 * time.Millisecond
	compiled, err := javascript.CompileExpression(`(() => { if (params.busy) { while (true) {} } return 42; })()`, limits)
	require.NoError(t, err)
	for _, callerDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint("caller deadline=", callerDeadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if callerDeadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
			}
			input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"busy": true}})}
			result, err := evaluateWithWatchdog(t, ctx, cancel, compiled, input)

			// The expression hit its policy limit while the caller remains active.
			require.ErrorIs(t, err, javascript.ErrLimitExceeded)
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			require.NoError(t, ctx.Err())
			require.Nil(t, result)

			// A timed-out evaluation cannot poison a later call on the same program.
			input.Context = expressionContext(t, render.Input{Params: map[string]any{"busy": false}})
			require.JSONEq(t, `42`, string(evaluateExpression(t, compiled, input)))
		})
	}
}

func TestExpressionStackAndOutputLimits(t *testing.T) {
	// Recursive authored code trips the configured stack bound before its time budget.
	compiled := compileExpression(t, `(() => { function recurse() { return recurse(); } return recurse(); })()`)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	result, err := compiled.Evaluate(context.Background(), input)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, result)

	// Byte limits apply to the encoded JSON result, including escaping and quotes.
	limits := expressionLimits()
	limits.MaxOutputBytes = len(`"\n"`)
	compiled, err = javascript.CompileExpression(`"\n"`, limits)
	require.NoError(t, err)
	require.Equal(t, `"\n"`, string(evaluateExpression(t, compiled, input)))
	limits.MaxOutputBytes--
	compiled, err = javascript.CompileExpression(`"\n"`, limits)
	require.NoError(t, err)
	result, err = compiled.Evaluate(context.Background(), input)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, result)
}

func TestExpressionInvalidArguments(t *testing.T) {
	// Invalid API arguments return errors rather than panicking or fabricating an invocation.
	compiled := compileExpression(t, `1`)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	_, err := compiled.Evaluate(nil, input)
	require.Error(t, err)
	_, err = compiled.Evaluate(context.Background(), javascript.Input{})
	require.Error(t, err)
	var absent *javascript.Expression
	_, err = absent.Evaluate(context.Background(), input)
	require.Error(t, err)
	_, err = new(javascript.Expression).Evaluate(context.Background(), input)
	require.Error(t, err)

	for _, response := range []string{`null`, `1`, `[]`, `"text"`, `{"a":1} {"b":2}`, strings.Repeat("x", 10)} {
		t.Run(response, func(t *testing.T) {
			// A supplied response must be exactly one JSON object, never a scalar or array.
			input.Response = common.RawJSON(response)
			result, err := compiled.Evaluate(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
