package javascript_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// evaluateTool checks that a successful invocation produces detached JSON.
func evaluateTool(t *testing.T, program *javascript.ToolProgram, input javascript.Input) common.RawJSON {
	t.Helper()
	result, err := program.Evaluate(context.Background(), input)
	require.NoError(t, err)
	require.True(t, json.Valid(result))
	return result
}

// TestToolEvaluateCapturedInput exercises published helpers, entry-point arguments, and copied aliases together.
func TestToolEvaluateCapturedInput(t *testing.T) {
	generation, catalog := uint64(7), uint64(11)
	input := javascript.Input{
		Context: expressionContext(t, render.Input{
			Params: map[string]any{
				"name":  "Ada",
				"count": 3,
			},
			Cfg: map[string]any{
				"tenant": "acme",
				"nested": map[string]any{
					"region": "us",
				},
			},
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

	// Publication initializes helpers without invocation data, so cfg is read only when called.
	library, err := apjs.CompileAndValidateLibrary(`
		let calls = 0;
		function greet(name) { calls++; return name + "@" + cfg.tenant; }
	`)
	require.NoError(t, err)

	program := compileTool(t, `
		async function execute(args, snapshot) {
			const result = {
				greeting: greet(args.name), 
                count: args.count, 
                calls: calls,
				connection: snapshot.connection, 
                tool: snapshot.tool,
				region: cfg.nested.region, 
                label: labels.team, 
                annotation: annotations.note,
				responseID: response.body.id,
				aliases: args === params && snapshot === context && cfg === context.cfg &&
					labels === context.labels && annotations === context.annotations,
				paramsSeparate: !("params" in snapshot),
				nativeObjects: Object.getPrototypeOf(args) === Object.prototype &&
					Object.getPrototypeOf(cfg) === Object.prototype
			};

			args.name = "changed"; 
            args.count = 99;
			cfg.nested.region = "changed"; 
            labels.team = "changed";
			annotations.note = "changed"; 
            response.body.id = "changed";

			return result;
		}
	`, library)

	// The complete Tool must also validate before any connection snapshot is supplied.
	require.NoError(t, program.Validate(context.Background()))

	// Every invocation starts from the captured data and a fresh helper-library state.
	for range 2 {
		result := evaluateTool(t, program, input)
		require.JSONEq(t, `{
			"greeting": "Ada@acme",
            "count": 3,
            "calls": 1,
			"connection": {
              "id": "cxn_example",
              "namespace": "root.product.owner"
             },
			"tool": {
               "id": "tol_example",
               "revision": 9,
               "toolSetId": "tls_example",
               "toolSetGeneration": 7,
               "catalogRevision": 11
            },
			"region": "us",
            "label": "blue",
            "annotation": "captured",
            "responseID": "contact-1",
			"aliases": true,
            "paramsSeparate": true,
            "nativeObjects": true
		}`, string(result))

		// Mutating returned bytes also cannot affect a later invocation.
		result[0] = 'x'
	}

	require.JSONEq(t, `{"status":200,"body":{"id":"contact-1"}}`, string(input.Response))
}

// TestToolEvaluateConcurrentRuntimes keeps helper and input mutations private to each invocation.
func TestToolEvaluateConcurrentRuntimes(t *testing.T) {
	library := toolLibrary(t, `let calls = 0; function next() { return ++calls; }`)
	program := compileTool(t, `async function execute(args) {
		args.count++;
		return {count: args.count, calls: await Promise.resolve(next())};
	}`, library)
	input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"count": 3}})}

	for i := range 8 {
		t.Run(fmt.Sprintf("invocation-%d", i), func(t *testing.T) {
			t.Parallel()
			// Sharing the compiled program and immutable snapshot must not share VM state.
			require.JSONEq(t, `{"count":4,"calls":1}`, string(evaluateTool(t, program, input)))
		})
	}
}

// TestToolEvaluatePromiseJobs drains language-level asynchronous continuations before encoding.
func TestToolEvaluatePromiseJobs(t *testing.T) {
	for name, body := range map[string]string{
		"direct return": `return 42;`,
		"await":         `return await Promise.resolve(42);`,
		"then chain":    `return Promise.resolve(20).then(x => x + 1).then(x => x * 2);`,
		"nested jobs": `let resolve; const pending = new Promise(r => { resolve = r; });
			Promise.resolve().then(() => Promise.resolve().then(() => resolve(42)));
			return await pending;`,
		"thenable": `return await {then(resolve) { resolve(42); }};`,
	} {
		t.Run(name, func(t *testing.T) {
			// Await and then jobs are driven by the same VM that owns the entry point.
			program := compileTool(t, "async function execute() {"+body+"}", nil)
			input := javascript.Input{Context: expressionContext(t, render.Input{})}
			require.JSONEq(t, `42`, string(evaluateTool(t, program, input)))
		})
	}
}

// TestToolEvaluateUnhandledRejections detects abandoned failures without exposing their values.
func TestToolEvaluateUnhandledRejections(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"helper initialization": {`async function execute() { return 1; }`, `Promise.reject("private-rejection");`},
		"tool initialization":   {`Promise.reject("private-rejection"); async function execute() { return 1; }`, ""},
		"entry point":           {`async function execute() { Promise.reject("private-rejection"); return 1; }`, ""},
		"continuation":          {`async function execute() { Promise.resolve().then(() => { throw "private-rejection"; }); return 1; }`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// An otherwise successful return cannot conceal an unhandled promise rejection.
			program := compileTool(t, test.source, toolLibrary(t, test.library))
			result, err := program.Evaluate(context.Background(), javascript.Input{Context: expressionContext(t, render.Input{})})
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private-rejection")
			require.Nil(t, result)
		})
	}

	// A rejection that gains a handler before the job queue settles is not abandoned.
	program := compileTool(t, `async function execute() {
		const rejected = Promise.reject("private-rejection");
		await 0;
		return rejected.catch(() => 42);
	}`, nil)
	require.JSONEq(t, `42`, string(evaluateTool(t, program, javascript.Input{Context: expressionContext(t, render.Input{})})))
}

// TestToolEvaluateRejectsFailedInitialization enforces the same boundary as publication validation.
func TestToolEvaluateRejectsFailedInitialization(t *testing.T) {
	program := compileTool(t, `
		const early = Promise.reject("private-initialization-rejection");
		async function execute() {
			early.catch(() => {});
			return true;
		}
	`, nil)

	// Publication rejects an abandoned initialization promise without invoking execute.
	err := program.Validate(context.Background())
	require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
	require.Equal(t, apjsruntime.ErrEvaluation.Error(), err.Error())

	// Invocation must reject that same initialization before execute can attach a handler.
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	result, err := program.Evaluate(context.Background(), input)
	require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
	require.Equal(t, apjsruntime.ErrEvaluation.Error(), err.Error())
	require.Nil(t, result)
}

// TestToolEvaluateSanitizesErrors covers throws and rejections during each authored execution phase.
func TestToolEvaluateSanitizesErrors(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"helper throws":         {`async function execute() { return 1; }`, `throw new Error("private-initialization-value");`},
		"initialization throws": {`throw new Error("private-initialization-value"); async function execute() { return 1; }`, ""},
		"execute throws":        {`async function execute() { throw new Error(cfg.private); }`, ""},
		"throws string":         {`async function execute() { throw cfg.private; }`, ""},
		"throws object":         {`async function execute() { throw {toString() { throw cfg.private; }}; }`, ""},
		"await rejection":       {`async function execute() { return await Promise.reject(cfg.private); }`, ""},
		"sync replacement":      {`async function execute() {} execute = () => 1;`, ""},
		// Spoof the async prototype so initialization passes and the result guard
		// must reject the non-Promise without traversing its infinite getter.
		"thenable replacement": {`async function execute() {}
			execute = Object.setPrototypeOf(
				() => ({get then() { for (;;) {} }}), Object.getPrototypeOf(execute));`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// Neither JavaScript stacks nor configuration-derived exception details leave the VM.
			program := compileTool(t, test.source, toolLibrary(t, test.library))
			input := javascript.Input{Context: expressionContext(t, render.Input{Cfg: map[string]any{"private": "private-connection-value"}})}
			result, err := program.Evaluate(context.Background(), input)
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.Equal(t, apjsruntime.ErrEvaluation.Error(), err.Error())
			require.Nil(t, result)
		})
	}
}

// TestToolEvaluatePendingPromiseFailsPromptly rejects promises that no installed host can settle.
func TestToolEvaluatePendingPromiseFailsPromptly(t *testing.T) {
	limits := expressionLimits()
	limits.Timeout = 5 * time.Second
	program, err := javascript.CompileTool(`async function execute() { return await new Promise(() => {}); }`, nil, limits)
	require.NoError(t, err)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}

	// Bound the test externally so a regression cannot leave it waiting for host work.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// With no host work available, waiting for either deadline cannot make progress.
	start := time.Now()
	result, err := program.Evaluate(ctx, input)
	require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
	require.Nil(t, result)
	require.Less(t, time.Since(start), time.Second)
}

// TestToolEvaluateJSONContract reuses strict output semantics after the entry-point promise settles.
func TestToolEvaluateJSONContract(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	for name, value := range map[string]string{
		"undefined": `undefined`, "function": `() => 1`, "nonfinite": `NaN`,
		"bigint": `1n`, "nested promise": `({value: Promise.resolve(1)})`,
		"date": `new Date(0)`, "getter": `({get value() { throw "private"; }})`,
		"proxy": `new Proxy({}, {ownKeys() { throw "private"; }})`,
		"cycle": `(() => { const value = {}; value.self = value; return value; })()`,
	} {
		t.Run(name, func(t *testing.T) {
			// Unsupported values are rejected without coercion or authored property access.
			program := compileTool(t, "async function execute() { return "+value+"; }", nil)
			result, err := program.Evaluate(context.Background(), input)
			require.ErrorIs(t, err, apjsruntime.ErrInvalidOutput)
			require.Equal(t, apjsruntime.ErrInvalidOutput.Error(), err.Error())
			require.Nil(t, result)
		})
	}

	// Capture conversion functions before helper initialization can poison the globals.
	library := toolLibrary(t, `
		Object.getOwnPropertyDescriptor = () => { throw "private"; };
		Reflect.ownKeys = () => { throw "private"; };
		JSON.stringify = () => { throw "private"; };
	`)
	program := compileTool(t, `async function execute() { return {values: [null, false, "雪", 42]}; }`, library)
	require.JSONEq(t, `{"values":[null,false,"雪",42]}`, string(evaluateTool(t, program, input)))
}

// TestToolEvaluateLimits keeps execution, initialization, and result conversion under one policy.
func TestToolEvaluateLimits(t *testing.T) {
	for name, test := range map[string]struct {
		source, library string
		change          func(*javascript.Limits)
	}{
		"execute loop": {`async function execute() { for (;;) {} }`, "", func(l *javascript.Limits) { l.Timeout = 100 * time.Millisecond }},
		"job loop":     {`async function execute() { return Promise.resolve().then(() => { for (;;) {} }); }`, "", func(l *javascript.Limits) { l.Timeout = 100 * time.Millisecond }},
		"endless microtasks": {`async function execute() {
			const again = () => Promise.resolve().then(again);
			return again();
		}`, "", func(l *javascript.Limits) { l.Timeout = 100 * time.Millisecond }},
		"helper loop":  {`async function execute() { return 1; }`, `for (;;) {}`, func(l *javascript.Limits) { l.Timeout = 100 * time.Millisecond }},
		"stack":        {`function recurse() { return recurse(); } async function execute() { return recurse(); }`, "", func(l *javascript.Limits) { l.MaxStackDepth = 16 }},
		"input bytes":  {`async function execute() { return 1; }`, "", func(l *javascript.Limits) { l.MaxInputBytes = 16 }},
		"output bytes": {`async function execute() { return "x".repeat(100); }`, "", func(l *javascript.Limits) { l.MaxOutputBytes = 32 }},
		"output nodes": {`async function execute() { return new Array(100).fill(1); }`, "", func(l *javascript.Limits) { l.MaxNodes = 32 }},
		"output depth": {`async function execute() { return [[[[[[1]]]]]]; }`, "", func(l *javascript.Limits) { l.MaxDepth = 4 }},
	} {
		t.Run(name, func(t *testing.T) {
			// Lower one cap while leaving the other limits sufficient for this scenario.
			limits := expressionLimits()
			test.change(&limits)
			program, err := javascript.CompileTool(test.source, toolLibrary(t, test.library), limits)
			require.NoError(t, err)
			result, err := program.Evaluate(context.Background(), javascript.Input{Context: expressionContext(t, render.Input{})})

			// All phases discard partial output on a policy failure.
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestToolEvaluateCallerCancellation retains caller errors instead of reclassifying them as policy failures.
func TestToolEvaluateCallerCancellation(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	program := compileTool(t, `async function execute() { for (;;) {} }`, nil)

	// A canceled caller takes precedence before authored code begins.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := program.Evaluate(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	// A caller deadline also interrupts executing JavaScript and retains its classification.
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err = program.Evaluate(ctx, input)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}

// TestToolEvaluateNoHostIO keeps the currently supported entry-point runner free of host capabilities.
func TestToolEvaluateNoHostIO(t *testing.T) {
	program := compileTool(t, `async function execute() {
		return {fetch: typeof fetch, require: typeof require, process: typeof process,
			setTimeout: typeof setTimeout, WebSocket: typeof WebSocket,
			sourceMaps: await eval("7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"),
			functionSourceMaps: (new Function("return 8\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"))()};
	}`, nil)

	// Dynamic parsing stays local even after await, and no network/timer/module API is installed.
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	require.JSONEq(t, `{"fetch":"undefined","require":"undefined","process":"undefined","setTimeout":"undefined","WebSocket":"undefined","sourceMaps":7,"functionSourceMaps":8}`, string(evaluateTool(t, program, input)))
}

// TestToolEvaluateInvalidInputs rejects incomplete invocation state before running authored code.
func TestToolEvaluateInvalidInputs(t *testing.T) {
	program := compileTool(t, `async function execute() { throw "must not run"; }`, nil)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	for name, test := range map[string]struct {
		program *javascript.ToolProgram
		ctx     context.Context
		input   javascript.Input
	}{
		"nil program":        {nil, context.Background(), input},
		"empty program":      {&javascript.ToolProgram{}, context.Background(), input},
		"nil context":        {program, nil, input},
		"missing snapshot":   {program, context.Background(), javascript.Input{}},
		"malformed response": {program, context.Background(), javascript.Input{Context: input.Context, Response: common.RawJSON(`{"private":`)}},
	} {
		t.Run(name, func(t *testing.T) {
			// Invalid Go inputs must produce a safe error rather than a panic or script exception.
			result, err := test.program.Evaluate(test.ctx, test.input)
			require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}
}
