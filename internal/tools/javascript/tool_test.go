package javascript_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/stretchr/testify/require"
)

// compileTool prepares an entry point without running its library or source.
func compileTool(t *testing.T, source string, library *apjs.Library) *javascript.ToolProgram {
	t.Helper()
	program, err := javascript.CompileTool(source, library, expressionLimits())
	require.NoError(t, err)
	return program
}

// toolLibrary deliberately avoids publication validation, which runs its source.
func toolLibrary(t *testing.T, source string) *apjs.Library {
	t.Helper()
	library, err := apjs.CompileLibrary(source)
	require.NoError(t, err)
	return library
}

// toolValue calls an initialized async entry point and reads its settled result.
func toolValue(t *testing.T, execute goja.Callable) goja.Value {
	t.Helper()
	value, err := execute(goja.Undefined())
	require.NoError(t, err)
	promise, ok := value.Export().(*goja.Promise)
	require.True(t, ok, "an async entry point returns a Promise")
	require.Equal(t, goja.PromiseStateFulfilled, promise.State())
	return promise.Result()
}

// TestToolCompileDoesNotInitializeCode keeps publication parsing separate from initialization.
func TestToolCompileDoesNotInitializeCode(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		library string
	}{
		"tool initialization":   {`throw new Error("private-tool-source"); async function execute() {}`, ""},
		"helper initialization": {`async function execute() {}`, `throw new Error("private-helper-source")`},
	} {
		t.Run(name, func(t *testing.T) {
			// Compiling source succeeds without observing either top-level throw.
			program := compileTool(t, test.source, toolLibrary(t, test.library))

			// Bounded initialization classifies the failure without exposing its text.
			err := program.Validate(context.Background())
			require.ErrorIs(t, err, javascript.ErrEvaluation)
			require.NotContains(t, err.Error(), "private-")
		})
	}
}

// TestToolIgnoresSourceMapResources prevents parser directives from loading files.
func TestToolIgnoresSourceMapResources(t *testing.T) {
	// Authored debugging directives are not publication-time filesystem access.
	program := compileTool(t, `async function execute() {}
//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json`, nil)
	require.NoError(t, program.Validate(context.Background()))

	// Dynamically parsed initialization code uses the same no-resource parser policy.
	program = compileTool(t, `
		eval("1\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json");
		async function execute() {
			return eval("7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json");
		}
	`, nil)
	require.NoError(t, program.Validate(context.Background()))

	// Initialize also disables a caller-supplied loader before any authored code runs.
	vm := goja.New()
	loads := 0
	vm.SetParserOptions(parser.WithSourceMapLoader(func(string) ([]byte, error) {
		loads++
		return nil, fmt.Errorf("source maps must not be loaded")
	}))
	execute, err := program.Initialize(vm)
	require.NoError(t, err)
	require.Equal(t, int64(7), toolValue(t, execute).ToInteger())
	require.Zero(t, loads)
}

// TestToolRequiresDirectAsyncEntrypoint rejects ambiguous or unsupported entry points.
func TestToolRequiresDirectAsyncEntrypoint(t *testing.T) {
	for name, source := range map[string]string{
		"missing":          `const helper = 1;`,
		"sync":             `function execute(params, context) { return params; }`,
		"generator":        `async function* execute() { yield 1; }`,
		"arrow":            `const execute = async () => 1;`,
		"nested":           `function helper() { async function execute() {} }`,
		"block scoped":     `{ async function execute() {} }`,
		"duplicate":        `async function execute() {} async function execute() {}`,
		"malformed":        `async function execute(`,
		"invalid encoding": string([]byte{0xff}) + `async function execute() {}`,
		"empty":            " \n ",
	} {
		t.Run(name, func(t *testing.T) {
			// Only an unambiguous top-level async function declaration is accepted.
			program, err := javascript.CompileTool(source, nil, expressionLimits())
			require.ErrorIs(t, err, javascript.ErrInvalidSource)
			require.Nil(t, program)
		})
	}

	// Parameter spelling and count are ordinary JavaScript, not an extra contract.
	program := compileTool(t, `async function execute(input, snapshot, ...rest) { return 1; }`, nil)
	require.NoError(t, program.Validate(context.Background()))
}

// TestToolRejectsReservedGlobalDeclarations catches claims even without assigned values.
func TestToolRejectsReservedGlobalDeclarations(t *testing.T) {
	for _, name := range []string{
		"fetch", "URL", "URLSearchParams", "TextEncoder", "TextDecoder", "atob", "btoa",
		"console", "ToolError", "toolResult", "params", "context", "cfg", "labels", "annotations", "response", "data",
	} {
		t.Run(name, func(t *testing.T) {
			// An undefined declaration still claims a host name and must be rejected.
			program, err := javascript.CompileTool("var "+name+"; async function execute() {}", nil, expressionLimits())
			require.ErrorIs(t, err, javascript.ErrInvalidSource)
			require.Nil(t, program)
		})
	}

	for name, declaration := range map[string]string{
		"constant":        `const fetch = undefined;`,
		"lexical":         `let fetch;`,
		"class":           `class fetch {}`,
		"function":        `function fetch() {}`,
		"block var":       `if (false) { var fetch; }`,
		"loop var":        `for (var fetch of []) {}`,
		"object binding":  `const {value: fetch} = {};`,
		"array binding":   `const [fetch] = [];`,
		"rest binding":    `const {...fetch} = {};`,
		"escaped binding": `var f\u0065tch;`,
	} {
		t.Run(name, func(t *testing.T) {
			// Syntax-level checks catch hoisting and nested binding targets before execution.
			program, err := javascript.CompileTool(declaration+` async function execute() {}`, nil, expressionLimits())
			require.ErrorIs(t, err, javascript.ErrInvalidSource)
			require.Nil(t, program)
		})
	}
}

// TestToolChecksLibraryGlobalsWithoutChangingConnectorPolicy scopes the extra restrictions to Tools.
func TestToolChecksLibraryGlobalsWithoutChangingConnectorPolicy(t *testing.T) {
	for name, source := range map[string]string{
		"host variable":           `var fetch;`,
		"host lexical":            `const URL = undefined;`,
		"hoisted variable":        `if (false) { var console; }`,
		"destructuring":           `const {value: toolResult} = {};`,
		"entry point":             `function execute() {}`,
		"entry point lexical":     `const execute = 1;`,
		"conditional function":    `if (true) function fetch() {}`,
		"conditional entry point": `if (true) function execute() {}`,
		"conditional in block":    `{ if (true) function fetch() {} }`,
		"conditional in function": `function helper() { if (true) function fetch() {} }`,
		"conditional in try":      `try { if (true) function fetch() {} } catch (err) {}`,
		"conditional in switch":   `switch (1) { case 1: if (true) function fetch() {} }`,
	} {
		t.Run(name, func(t *testing.T) {
			// Connector compilation remains permissive; Tool use adds host-name checks.
			library := toolLibrary(t, source)
			program, err := javascript.CompileTool(`async function execute() {}`, library, expressionLimits())
			require.ErrorIs(t, err, javascript.ErrInvalidSource)
			require.Nil(t, program)
		})
	}

	// A local binding or property named like a host global does not claim that global.
	library := toolLibrary(t, `
		const {fetch: helper} = {fetch: 3};
		function local(fetch) { const URL = fetch + helper; return URL; }
		{ function fetch() { return "block-local"; } }
	`)
	program := compileTool(t, `
		async function execute(params, context) {
			const fetch = 4;
			return local(fetch);
		}
	`, library)
	execute, err := program.Initialize(goja.New())
	require.NoError(t, err)
	require.Equal(t, int64(7), toolValue(t, execute).ToInteger())
}

// TestToolReusesHelperProgramInFreshRuntimes exercises shared programs with isolated state.
func TestToolReusesHelperProgramInFreshRuntimes(t *testing.T) {
	library := toolLibrary(t, `
		const prefix = "contact:";
		let count = 0;
		function next() { count++; return prefix + count; }
	`)
	program := compileTool(t, `async function execute() { return await Promise.resolve(next()); }`, library)

	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprintf("runtime-%d", i), func(t *testing.T) {
			t.Parallel()
			// The immutable programs can be shared; each VM owns its helper state.
			execute, err := program.Initialize(goja.New())
			require.NoError(t, err)
			require.Equal(t, "contact:1", toolValue(t, execute).String())
			require.Equal(t, "contact:2", toolValue(t, execute).String())

			// Validation uses another private VM without mutating this invocation's state.
			require.NoError(t, program.Validate(context.Background()))
			require.Equal(t, "contact:3", toolValue(t, execute).String())
		})
	}
}

// TestToolInitializationPreservesHostBindings keeps a caller-installed capability intact.
func TestToolInitializationPreservesHostBindings(t *testing.T) {
	vm := goja.New()
	calls := 0
	require.NoError(t, vm.Set("fetch", func() string { calls++; return "host-response" }))
	library := toolLibrary(t, `delete globalThis.fetch; globalThis.fetch = () => "replacement";`)
	program := compileTool(t, `async function execute() { return fetch(); }`, library)

	// Initialization protects the host before sloppy helper assignments, without calling execute.
	execute, err := program.Initialize(vm)
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Equal(t, "host-response", toolValue(t, execute).String())
	require.Equal(t, 1, calls)

	// Host globals are immutable before either helper or Tool initialization runs.
	_, err = vm.RunString(`"use strict"; fetch = () => "replacement";`)
	require.Error(t, err)
	require.Equal(t, "host-response", toolValue(t, execute).String())
	require.Equal(t, 2, calls)
}

// TestToolValidationHasNoIOAndDoesNotCallEntrypoint validates definitions without exercising actions.
func TestToolValidationHasNoIOAndDoesNotCallEntrypoint(t *testing.T) {
	// A body that would call the network and throw is never invoked by validation.
	program := compileTool(t, `
		async function execute() {
			await fetch("https://example.invalid/private");
			throw new Error("private-body");
		}
	`, nil)
	require.NoError(t, program.Validate(context.Background()))

	// Top-level code cannot obtain network access while validating a definition.
	program = compileTool(t, `fetch("https://example.invalid/private"); async function execute() {}`, nil)
	err := program.Validate(context.Background())
	require.ErrorIs(t, err, javascript.ErrEvaluation)
	require.NotContains(t, err.Error(), "private")

	// Initialization can replace the declaration, so callability is checked afterward.
	program = compileTool(t, `async function execute() {} execute = 42;`, nil)
	err = program.Validate(context.Background())
	require.ErrorIs(t, err, javascript.ErrEvaluation)
}

// TestToolCombinedSourceBudget accounts for both programs and retains a private policy copy.
func TestToolCombinedSourceBudget(t *testing.T) {
	librarySource := `const prefix = "雪";`
	source := `async function execute() { return prefix; }`
	library := toolLibrary(t, librarySource)
	limits := expressionLimits()
	limits.MaxSourceBytes = len(librarySource) + len(source)

	// The same byte allowance includes helper source, with an inclusive boundary.
	_, err := javascript.CompileTool(source, library, limits)
	require.NoError(t, err)
	limits.MaxSourceBytes--
	program, err := javascript.CompileTool(source, library, limits)
	require.ErrorIs(t, err, javascript.ErrLimitExceeded)
	require.Nil(t, program)

	// Caller-owned policy is copied when the reusable program is compiled.
	limits = expressionLimits()
	program, err = javascript.CompileTool(source, library, limits)
	require.NoError(t, err)
	limits.Timeout = time.Nanosecond
	limits.MaxStackDepth = 1
	require.NoError(t, program.Validate(context.Background()))
}

// TestToolValidationLimitsAndCancellation bounds initialization and preserves caller cancellation.
func TestToolValidationLimitsAndCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		library string
	}{
		"tool loop":   {`while (true) {} async function execute() {}`, ""},
		"helper loop": {`async function execute() {}`, `while (true) {}`},
	} {
		t.Run(name, func(t *testing.T) {
			// Compilation remains inert; initialization is interrupted by its policy timeout.
			limits := expressionLimits()
			limits.Timeout = 250 * time.Millisecond
			program, err := javascript.CompileTool(test.source, toolLibrary(t, test.library), limits)
			require.NoError(t, err)
			require.ErrorIs(t, program.Validate(context.Background()), javascript.ErrLimitExceeded)
		})
	}

	// Recursive helper initialization uses the same bounded stack as Tool source.
	program := compileTool(t, `async function execute() {}`, toolLibrary(t, `function recur() { recur(); } recur();`))
	require.ErrorIs(t, program.Validate(context.Background()), javascript.ErrLimitExceeded)

	// A caller deadline interrupts active initialization before the longer Tool policy.
	program = compileTool(t, `while (true) {} async function execute() {}`, nil)
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer deadlineCancel()
	require.ErrorIs(t, program.Validate(deadlineCtx), context.DeadlineExceeded)

	// Caller cancellation takes precedence over an initialization exception.
	program = compileTool(t, `throw new Error("private"); async function execute() {}`, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, program.Validate(ctx), context.Canceled)
	expired, expire := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer expire()
	require.ErrorIs(t, program.Validate(expired), context.DeadlineExceeded)
}

// TestToolRejectsMissingPolicyAndRuntime rejects incomplete calls without leaking source text.
func TestToolRejectsMissingPolicyAndRuntime(t *testing.T) {
	// Missing policy cannot disable the limits used during publication validation.
	program, err := javascript.CompileTool(`async function execute() {}`, nil, javascript.Limits{})
	require.ErrorIs(t, err, javascript.ErrInvalidInput)
	require.Nil(t, program)

	// A compiled program still needs a caller-owned runtime or validation context.
	program = compileTool(t, `async function execute() {}`, nil)
	_, err = program.Initialize(nil)
	require.ErrorIs(t, err, javascript.ErrInvalidInput)
	require.ErrorIs(t, program.Validate(nil), javascript.ErrInvalidInput)

	// Compilation errors reveal no authored source even when syntax is private.
	_, err = javascript.CompileTool(`async function execute( private-source-marker`, nil, expressionLimits())
	require.ErrorIs(t, err, javascript.ErrInvalidSource)
	require.False(t, strings.Contains(err.Error(), "private-source-marker"))
}
