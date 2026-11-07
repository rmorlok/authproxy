package javascript_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
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

// TestToolRejectsReservedGlobalDeclarations catches claims even without assigned values.
func TestToolRejectsReservedGlobalDeclarations(t *testing.T) {
	for _, name := range []string{
		"fetch", "URL", "URLSearchParams", "TextEncoder", "TextDecoder", "atob", "btoa",
		"console", "ToolError", "toolResult", "params", "context", "cfg", "labels", "annotations", "response", "data",
	} {
		t.Run(name, func(t *testing.T) {
			// An undefined declaration still claims a host name and must be rejected.
			program, err := javascript.CompileTool("var "+name+"; async function execute() {}", nil, expressionLimits())
			require.ErrorIs(t, err, apjsruntime.ErrInvalidSource)
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
			require.ErrorIs(t, err, apjsruntime.ErrInvalidSource)
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
			require.ErrorIs(t, err, apjsruntime.ErrInvalidSource)
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

// TestToolInitializationPreservesHostBindings keeps a caller-installed capability intact.
func TestToolInitializationPreservesHostBindings(t *testing.T) {
	vm := goja.New()
	calls := 0
	require.NoError(t, vm.Set("fetch", func() string { calls++; return "host-response" }))
	// Mirror captured input bindings: they are already immutable and enumerable.
	inputNames := []string{"params", "context", "response", "cfg", "labels", "annotations"}
	for _, name := range inputNames {
		require.NoError(t, vm.GlobalObject().DefineDataProperty(
			name, vm.NewObject(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE,
		))
	}
	library := toolLibrary(t, `delete globalThis.fetch; globalThis.fetch = () => "replacement";`)
	program := compileTool(t, `async function execute() { return fetch(); }`, library)

	// Initialization protects the host before sloppy helper assignments, without calling execute.
	execute, err := program.Initialize(vm)
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Equal(t, "host-response", toolValue(t, execute).String())
	require.Equal(t, 1, calls)

	// Capabilities stay hidden from enumeration; existing input visibility is preserved.
	require.NotContains(t, vm.GlobalObject().Keys(), "fetch")
	for _, name := range inputNames {
		require.Contains(t, vm.GlobalObject().Keys(), name)
	}

	// Host globals are immutable before either helper or Tool initialization runs.
	_, err = vm.RunString(`"use strict"; fetch = () => "replacement";`)
	require.Error(t, err)
	require.Equal(t, "host-response", toolValue(t, execute).String())
	require.Equal(t, 2, calls)
}

// TestToolRejectsMissingPolicyAndRuntime rejects incomplete calls without leaking source text.
func TestToolRejectsMissingPolicyAndRuntime(t *testing.T) {
	// Missing policy cannot disable the limits used during publication validation.
	program, err := javascript.CompileTool(`async function execute() {}`, nil, javascript.Limits{})
	require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
	require.Nil(t, program)

	// A compiled program still needs a caller-owned runtime or validation context.
	program = compileTool(t, `async function execute() {}`, nil)
	_, err = program.Initialize(nil)
	require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
	require.ErrorIs(t, program.Validate(nil), apjsruntime.ErrInvalidInput)

	// Compilation errors reveal no authored source even when syntax is private.
	_, err = javascript.CompileTool(`async function execute( private-source-marker`, nil, expressionLimits())
	require.ErrorIs(t, err, apjsruntime.ErrInvalidSource)
	require.False(t, strings.Contains(err.Error(), "private-source-marker"))
}
