package runtime_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// compiledAsyncLibrary exercises the shared interface independently of connector packages.
// Its source and immutable program describe the same non-strict helper code.
type compiledAsyncLibrary struct {
	source  string        // Authored bytes used for combined source accounting.
	program *goja.Program // Reused only across distinct caller-owned runtimes.
}

// Source returns the exact helper source; a typed nil library is empty.
func (l *compiledAsyncLibrary) Source() string {
	if l == nil {
		return ""
	}
	return l.source
}

// Run initializes the helper in the caller's VM without validating or invoking execute.
func (l *compiledAsyncLibrary) Run(vm *goja.Runtime) error {
	if l == nil || l.program == nil {
		return nil
	}
	_, err := vm.RunProgram(l.program)
	return err
}

// asyncLibrary compiles helper code without running initialization or importing a domain adapter.
func asyncLibrary(t *testing.T, source string) *compiledAsyncLibrary {
	t.Helper()
	parsed, err := goja.Parse("helpers.js", source, parser.WithDisableSourceMaps)
	require.NoError(t, err)
	program, err := goja.CompileAST(parsed, false)
	require.NoError(t, err)
	return &compiledAsyncLibrary{source: source, program: program}
}

// asyncHostBindings describes this test host's globals explicitly; the compiler has no domain defaults.
func asyncHostBindings() []jsruntime.HostBinding {
	return []jsruntime.HostBinding{
		{Name: "fetch"},
		{Name: "params", PreserveEnumerable: true},
		{Name: "context", PreserveEnumerable: true},
	}
}

// compileAsync prepares an entry point without running its library or source.
func compileAsync(t *testing.T, source string, library jsruntime.CompiledLibrary) *jsruntime.AsyncProgram {
	t.Helper()
	program, err := jsruntime.CompileAsync(source, library, asyncHostBindings(), runtimeLimits())
	require.NoError(t, err)
	return program
}

// asyncValue calls an initialized entry point and reads its settled native Promise.
func asyncValue(t *testing.T, execute goja.Callable) goja.Value {
	t.Helper()
	value, err := execute(goja.Undefined())
	require.NoError(t, err)
	promise, ok := value.Export().(*goja.Promise)
	require.True(t, ok, "an async entry point returns a Promise")
	require.Equal(t, goja.PromiseStateFulfilled, promise.State())
	return promise.Result()
}

// TestAsyncCompileDoesNotInitializeCode keeps publication parsing separate from initialization.
func TestAsyncCompileDoesNotInitializeCode(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		library string
	}{
		"program initialization": {`throw new Error("private-tool-source"); async function execute() {}`, ""},
		"helper initialization":  {`async function execute() {}`, `throw new Error("private-helper-source")`},
	} {
		t.Run(name, func(t *testing.T) {
			// Compiling source succeeds without observing either top-level throw.
			program := compileAsync(t, test.source, asyncLibrary(t, test.library))

			// Bounded initialization classifies the failure without exposing its text.
			err := program.Validate(context.Background())
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private-")
		})
	}
}

// TestAsyncIgnoresSourceMapResources prevents parser directives from loading files.
func TestAsyncIgnoresSourceMapResources(t *testing.T) {
	// Authored debugging directives are not publication-time filesystem access.
	program := compileAsync(t, `async function execute() {}
//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json`, nil)
	require.NoError(t, program.Validate(context.Background()))

	// Dynamically parsed initialization code uses the same no-resource parser policy.
	program = compileAsync(t, `
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
	require.Equal(t, int64(7), asyncValue(t, execute).ToInteger())
	require.Zero(t, loads)
}

// TestAsyncRequiresDirectAsyncEntrypoint rejects ambiguous or unsupported entry points.
func TestAsyncRequiresDirectAsyncEntrypoint(t *testing.T) {
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
			program, err := jsruntime.CompileAsync(source, nil, asyncHostBindings(), runtimeLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
			require.Nil(t, program)
		})
	}

	// Parameter spelling and count are ordinary JavaScript, not an extra contract.
	program := compileAsync(t, `async function execute(input, snapshot, ...rest) { return 1; }`, nil)
	require.NoError(t, program.Validate(context.Background()))
}

// TestAsyncReusesHelperProgramInFreshRuntimes exercises shared programs with isolated state.
func TestAsyncReusesHelperProgramInFreshRuntimes(t *testing.T) {
	library := asyncLibrary(t, `
		const prefix = "contact:";
		let count = 0;
		function next() { count++; return prefix + count; }
	`)
	program := compileAsync(t, `async function execute() { return await Promise.resolve(next()); }`, library)

	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprintf("runtime-%d", i), func(t *testing.T) {
			t.Parallel()
			// The immutable programs can be shared; each VM owns its helper state.
			execute, err := program.Initialize(goja.New())
			require.NoError(t, err)
			require.Equal(t, "contact:1", asyncValue(t, execute).String())
			require.Equal(t, "contact:2", asyncValue(t, execute).String())

			// Validation uses another private VM without mutating this invocation's state.
			require.NoError(t, program.Validate(context.Background()))
			require.Equal(t, "contact:3", asyncValue(t, execute).String())
		})
	}
}

// TestAsyncValidationHasNoIOAndDoesNotCallEntrypoint validates definitions without exercising actions.
func TestAsyncValidationHasNoIOAndDoesNotCallEntrypoint(t *testing.T) {
	// A body that would call the network and throw is never invoked by validation.
	program := compileAsync(t, `
		async function execute() {
			await fetch("https://example.invalid/private");
			throw new Error("private-body");
		}
	`, nil)
	require.NoError(t, program.Validate(context.Background()))

	// Top-level code cannot obtain network access while validating a definition.
	program = compileAsync(t, `fetch("https://example.invalid/private"); async function execute() {}`, nil)
	err := program.Validate(context.Background())
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
	require.NotContains(t, err.Error(), "private")

	// Initialization can replace the declaration, so callability is checked afterward.
	program = compileAsync(t, `async function execute() {} execute = 42;`, nil)
	err = program.Validate(context.Background())
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
}

// TestAsyncValidationTracksRejections checks initialization promises without invoking the entry point.
func TestAsyncValidationTracksRejections(t *testing.T) {
	for name, test := range map[string]struct {
		source, library string
		wantErr         error
	}{
		"program rejection":       {`Promise.reject("private-rejection");`, "", jsruntime.ErrEvaluation},
		"library rejection":       {"", `Promise.reject("private-rejection");`, jsruntime.ErrEvaluation},
		"program reaction throws": {`Promise.resolve().then(() => { throw "private-rejection"; });`, "", jsruntime.ErrEvaluation},
		"library reaction throws": {"", `Promise.resolve().then(() => { throw "private-rejection"; });`, jsruntime.ErrEvaluation},
		"program catches":         {`Promise.reject("private-rejection").catch(() => {});`, "", nil},
		"library catches":         {"", `Promise.reject("private-rejection").catch(() => {});`, nil},
		"program catches in microtask": {`const failure = Promise.reject("private-rejection");
			Promise.resolve().then(() => failure.catch(() => {}));`, "", nil},
		"library catches in microtask": {"", `const failure = Promise.reject("private-rejection");
			Promise.resolve().then(() => failure.catch(() => {}));`, nil},
		"entry point stays uncalled": {"", "", nil},
	} {
		t.Run(name, func(t *testing.T) {
			// Only initialization executes; calling this body would introduce an unhandled rejection.
			source := test.source + `
				async function execute() {
					Promise.reject("private-entry-point-rejection");
					throw new Error("private-entry-point-error");
				}
			`
			program := compileAsync(t, source, asyncLibrary(t, test.library))
			err := program.Validate(context.Background())

			// Rejections handled by the end of initialization are accepted; abandoned ones fail safely.
			if test.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.wantErr)
				require.Equal(t, jsruntime.ErrEvaluation.Error(), err.Error())
			}
		})
	}
}

// TestAsyncEntrypointMustRemainAsync rejects initialization that swaps out the declaration.
func TestAsyncEntrypointMustRemainAsync(t *testing.T) {
	for name, source := range map[string]string{
		"reassigned":     `async function execute() {} execute = function () { return 1; };`,
		"var initialize": `var execute = function () { return 1; }; async function execute() {}`,
		"arrow":          `async function execute() {} execute = () => 1;`,
		"generator":      `async function execute() {} execute = function* () {};`,
		"class":          `async function execute() {} execute = class {};`,
		"in a job":       `async function execute() {} Promise.resolve().then(() => { execute = () => 1; });`,
		"proxy trap": `async function execute() {}
			execute = new Proxy(execute, { getPrototypeOf() { throw new Error("private-trap"); } });`,
	} {
		t.Run(name, func(t *testing.T) {
			// Each replacement is callable or nearly so, but breaks the Promise contract.
			program := compileAsync(t, source, nil)
			err := program.Validate(context.Background())
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private")
		})
	}

	// Swapping in another async function still honors the Promise contract.
	program := compileAsync(t, `
		async function execute() { return 1; }
		execute = async () => 2;
	`, nil)
	entry, err := program.Initialize(goja.New())
	require.NoError(t, err)
	require.Equal(t, int64(2), asyncValue(t, entry).ToInteger())
}

// TestAsyncValidationRejectsUnhandledRejections surfaces failures from async initialization.
func TestAsyncValidationRejectsUnhandledRejections(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		library string
	}{
		"direct rejection": {`Promise.reject(new Error("private")); async function execute() {}`, ""},
		"async capability": {`(async () => { await fetch("https://example.invalid/private"); })(); async function execute() {}`, ""},
		"helper rejection": {`async function execute() {}`, `(async () => { throw new Error("private"); })();`},
		"chained failure":  {`Promise.resolve().then(() => { throw new Error("private"); }); async function execute() {}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// Async top-level failures reject after the script ends, without a sync throw.
			program := compileAsync(t, test.source, asyncLibrary(t, test.library))
			err := program.Validate(context.Background())
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private")
		})
	}

	// A rejection the author handles, even from a later job, is not a failure.
	for _, source := range []string{
		`Promise.reject(new Error("expected")).catch(() => {}); async function execute() {}`,
		`const failed = Promise.reject(new Error("expected"));
		 Promise.resolve().then(() => failed.catch(() => {}));
		 async function execute() {}`,
	} {
		require.NoError(t, compileAsync(t, source, nil).Validate(context.Background()))
	}
}

// TestAsyncCombinedSourceBudget accounts for both programs and retains a private policy copy.
func TestAsyncCombinedSourceBudget(t *testing.T) {
	librarySource := `const prefix = "雪";`
	source := `async function execute() { return prefix; }`
	library := asyncLibrary(t, librarySource)
	limits := runtimeLimits()
	limits.MaxSourceBytes = len(librarySource) + len(source)

	// The same byte allowance includes helper source, with an inclusive boundary.
	_, err := jsruntime.CompileAsync(source, library, asyncHostBindings(), limits)
	require.NoError(t, err)
	limits.MaxSourceBytes--
	program, err := jsruntime.CompileAsync(source, library, asyncHostBindings(), limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, program)

	// Caller-owned policy is copied when the reusable program is compiled.
	limits = runtimeLimits()
	program, err = jsruntime.CompileAsync(source, library, asyncHostBindings(), limits)
	require.NoError(t, err)
	limits.Timeout = time.Nanosecond
	limits.MaxStackDepth = 1
	require.NoError(t, program.Validate(context.Background()))
}

// TestAsyncValidationLimitsAndCancellation bounds initialization and preserves caller cancellation.
func TestAsyncValidationLimitsAndCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		library string
	}{
		"program loop": {`while (true) {} async function execute() {}`, ""},
		"helper loop":  {`async function execute() {}`, `while (true) {}`},
	} {
		t.Run(name, func(t *testing.T) {
			// Compilation remains inert; initialization is interrupted by its policy timeout.
			limits := runtimeLimits()
			limits.Timeout = 250 * time.Millisecond
			program, err := jsruntime.CompileAsync(test.source, asyncLibrary(t, test.library), asyncHostBindings(), limits)
			require.NoError(t, err)
			require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)
		})
	}

	// Recursive helper initialization uses the same bounded stack as program source.
	program := compileAsync(t, `async function execute() {}`, asyncLibrary(t, `function recur() { recur(); } recur();`))
	require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)

	// A caller deadline interrupts active initialization before the longer program policy.
	program = compileAsync(t, `while (true) {} async function execute() {}`, nil)
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer deadlineCancel()
	require.ErrorIs(t, program.Validate(deadlineCtx), context.DeadlineExceeded)

	// Caller cancellation takes precedence over an initialization exception.
	program = compileAsync(t, `throw new Error("private"); async function execute() {}`, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, program.Validate(ctx), context.Canceled)
	expired, expire := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer expire()
	require.ErrorIs(t, program.Validate(expired), context.DeadlineExceeded)
}

// TestAsyncHostBindingsAreCallerOwned preserves copied names and their enumeration policy.
func TestAsyncHostBindingsAreCallerOwned(t *testing.T) {
	bindings := []jsruntime.HostBinding{
		{Name: "custom", PreserveEnumerable: true},
		{Name: "capability"},
	}
	library := asyncLibrary(t, `globalThis.custom = "replacement"; globalThis.capability = "replacement";`)
	program, err := jsruntime.CompileAsync(`async function execute() { return custom + ":" + capability; }`, library, bindings, runtimeLimits())
	require.NoError(t, err)

	// Mutating the original descriptors cannot change the compiled host policy.
	bindings[0].Name = "different"
	bindings[0].PreserveEnumerable = false
	bindings[1].PreserveEnumerable = true
	vm := goja.New()
	require.NoError(t, vm.Set("custom", "input"))
	require.NoError(t, vm.Set("capability", "host"))
	execute, err := program.Initialize(vm)
	require.NoError(t, err)
	require.Equal(t, "input:host", asyncValue(t, execute).String())

	// Data bindings keep existing visibility; capabilities become non-enumerable.
	require.Contains(t, vm.GlobalObject().Keys(), "custom")
	require.NotContains(t, vm.GlobalObject().Keys(), "capability")
	_, err = vm.RunString(`"use strict"; custom = "changed";`)
	require.Error(t, err)
}

// TestAsyncRejectsInvalidHostBindings rejects malformed integration options before probing source.
func TestAsyncRejectsInvalidHostBindings(t *testing.T) {
	for name, bindings := range map[string][]jsruntime.HostBinding{
		"empty":                 {{Name: ""}},
		"duplicate":             {{Name: "custom"}, {Name: "custom", PreserveEnumerable: true}},
		"entry point":           {{Name: "execute"}},
		"escaped spelling":      {{Name: `\u0066etch`}},
		"multiple names":        {{Name: "one,two"}},
		"declaration injection": {{Name: "one; let two"}},
		"initializer":           {{Name: "one=1"}},
		"destructuring":         {{Name: "{one}"}},
		"keyword":               {{Name: "return"}},
		"invalid encoding":      {{Name: string([]byte{0xff})}},
	} {
		t.Run(name, func(t *testing.T) {
			// A host name must identify exactly the global that initialization freezes.
			program, err := jsruntime.CompileAsync(`async function execute() {}`, nil, bindings, runtimeLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.Nil(t, program)
		})
	}

	// Literal Unicode identifiers are valid names, unlike escaped alternative spellings.
	program, err := jsruntime.CompileAsync(`async function execute() { return typeof café; }`, nil,
		[]jsruntime.HostBinding{{Name: "café"}}, runtimeLimits())
	require.NoError(t, err)
	execute, err := program.Initialize(goja.New())
	require.NoError(t, err)
	require.Equal(t, "undefined", asyncValue(t, execute).String())
}

// TestAsyncReservesCustomNames applies the declaration checks to caller-selected globals.
func TestAsyncReservesCustomNames(t *testing.T) {
	bindings := []jsruntime.HostBinding{{Name: "customHost"}}
	for _, declaration := range []string{
		`var customHost;`, `let customHost;`, `function customHost() {}`,
		`const {value: customHost} = {};`, `if (false) { var customHost; }`,
	} {
		// Authored claims fail even when they never assign a value at runtime.
		program, err := jsruntime.CompileAsync(declaration+` async function execute() {}`, nil, bindings, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		require.Nil(t, program)
	}
	for _, source := range []string{
		`if (true) function customHost() {}`,
		`function helper() { if (true) function customHost() {} }`,
		`if (true) function execute() {}`,
	} {
		// Sloppy conditional declarations can reach a global even from nested helpers.
		program, err := jsruntime.CompileAsync(`async function execute() {}`, asyncLibrary(t, source), bindings, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		require.Nil(t, program)
	}

	// With no declared host globals, the primitive does not invent a domain policy.
	program, err := jsruntime.CompileAsync(`const fetch = 7; async function execute() { return fetch; }`, nil, nil, runtimeLimits())
	require.NoError(t, err)
	execute, err := program.Initialize(goja.New())
	require.NoError(t, err)
	require.Equal(t, int64(7), asyncValue(t, execute).ToInteger())
}

// TestAsyncNilLibraryAndInvalidArguments handles absent optional helpers and required runtime state.
func TestAsyncNilLibraryAndInvalidArguments(t *testing.T) {
	for _, library := range []jsruntime.CompiledLibrary{nil, (*compiledAsyncLibrary)(nil)} {
		// Both absent helpers and a nil-safe typed library behave as an empty library.
		program, err := jsruntime.CompileAsync(`async function execute() { return 7; }`, library, nil, runtimeLimits())
		require.NoError(t, err)
		require.NoError(t, program.Validate(context.Background()))
		execute, err := program.Initialize(goja.New())
		require.NoError(t, err)
		require.Equal(t, int64(7), asyncValue(t, execute).ToInteger())
		_, err = program.Initialize(nil)
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
		require.ErrorIs(t, program.Validate(nil), jsruntime.ErrInvalidInput)
	}

	// Missing policy and missing compiled programs fail without panicking.
	program, err := jsruntime.CompileAsync(`async function execute() {}`, nil, nil, jsruntime.Limits{})
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, program)
	for _, invalid := range []*jsruntime.AsyncProgram{nil, {}} {
		_, err := invalid.Initialize(goja.New())
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
		require.ErrorIs(t, invalid.Validate(context.Background()), jsruntime.ErrInvalidInput)
	}
}
