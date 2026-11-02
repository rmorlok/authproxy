package javascript

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/rmorlok/authproxy/internal/apjs"
)

// toolHostNames reserves the Tool host and captured-input bindings. Reserving
// these names does not implement the corresponding capability: publication
// validation leaves them undefined, and a later invocation host supplies them.
var toolHostNames = []string{
	"fetch",
	"URL",
	"URLSearchParams",
	"TextEncoder",
	"TextDecoder",
	"atob",
	"btoa",
	"console",
	"ToolError",
	"toolResult",
	"params",
	"context",
	"cfg",
	"labels",
	"annotations",
	"response",
	"data",
}

// ToolProgram owns a compiled Tool entry point and its optional compiled
// connector library. It retains no runtime or invocation values and can be
// initialized concurrently in separate, caller-owned Goja runtimes. Compilation
// runs no authored code; validation runs initialization without host capabilities
// and does not call the returned execute binding.
type ToolProgram struct {
	program *goja.Program // Strict Tool script, including its execute declaration.
	library *apjs.Library // Immutable connector helpers, preserving connector syntax.
	limits  Limits        // Captured source, execution, and input/output policy.
}

// CompileTool compiles a script with exactly one top-level async, non-generator
// function declaration named execute. Parameter names and arity are left to the
// author; invocation will supply arguments and captured context positionally.
// Other helpers and top-level initialization are allowed. This function never
// runs authored code; Validate checks initialization separately without I/O.
//
// The combined Tool and connector source must fit MaxSourceBytes. Global host
// declarations are rejected, including uninitialized and destructured bindings;
// ordinary function-local or block-local declarations remain valid. Legacy
// unbraced conditional declarations of reserved names are rejected anywhere in
// a library because they can reach the global object. A connector library cannot
// declare execute because that entry point belongs to the Tool.
func CompileTool(
	source string,
	library *apjs.Library,
	limits Limits,
) (*ToolProgram, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}

	librarySource := library.Source()
	if len(source) > limits.MaxSourceBytes || len(librarySource) > limits.MaxSourceBytes-len(source) {
		return nil, ErrLimitExceeded
	}

	if !utf8.ValidString(source) ||
		!utf8.ValidString(librarySource) ||
		strings.TrimSpace(source) == "" {
		return nil, ErrInvalidSource
	}

	parsed, err := parser.ParseFile(
		nil, // fileSet
		"tool.js",
		source,
		0, // mode
		parser.WithDisableSourceMaps,
	)
	if err != nil {
		return nil, ErrInvalidSource
	}

	entries := 0
	for _, statement := range parsed.Body {
		declaration, ok := statement.(*ast.FunctionDeclaration)
		if !ok || declaration.Function.Name == nil ||
			declaration.Function.Name.Name.String() != "execute" {
			continue
		}

		if !declaration.Function.Async || declaration.Function.Generator {
			return nil, ErrInvalidSource
		}

		entries++
	}

	if entries != 1 {
		return nil, ErrInvalidSource
	}

	program, err := goja.CompileAST(parsed, true)
	if err != nil {
		return nil, ErrInvalidSource
	}

	if err := checkToolHostBindings(
		source,
		true,  // strict
		false, // reserveExecute
	); err != nil {
		return nil, err
	}

	if err := checkToolHostBindings(
		librarySource,
		false, // strict
		true,  // reserveExecute
	); err != nil {
		return nil, err
	}

	return &ToolProgram{
		program: program,
		library: library,
		limits:  limits,
	}, nil
}

// checkToolHostBindings uses the JavaScript compiler's own scope rules to find
// conflicting global declarations. The appended lexical declaration is only a
// compile-time probe and is never executed. Appending preserves source directives
// such as "use strict" and avoids an incomplete hand-written binding walker.
func checkToolHostBindings(source string, strict, reserveExecute bool) error {
	names := strings.Join(toolHostNames, ",")
	if reserveExecute {
		names += ",execute"
	}
	parsed, err := parser.ParseFile(
		nil, // fileSet
		"tool-bindings.js",
		source+"\n;let "+names+";",
		0, // mode
		parser.WithDisableSourceMaps,
	)
	if err != nil {
		return ErrInvalidSource
	}

	if !strict && hasReservedConditionalFunction(parsed) {
		return ErrInvalidSource
	}

	if _, err := goja.CompileAST(parsed, strict); err != nil {
		return ErrInvalidSource
	}

	return nil
}

// Initialize runs connector initialization and then the Tool script in vm,
// returning its callable execute binding without invoking it. Compilation checks
// the async declaration; initialization checks that execute is still callable
// after authored top-level code runs. The caller must
// own a fresh runtime on one goroutine, install host capabilities and copied
// inputs first, and keep cancellation/time limits active through initialization
// and subsequent calls. Only Goja's Interrupt may run on another goroutine.
// The returned callable belongs exclusively to vm and must not outlive it.
//
// Host bindings are made nonwritable and nonconfigurable before authored code
// runs; missing capabilities remain undefined. Stack policy is applied here.
// Top-level errors and noncallable entry points return fixed classifications,
// never source, thrown values, or library diagnostics. Discard vm on failure.
func (p *ToolProgram) Initialize(vm *goja.Runtime) (goja.Callable, error) {
	if p == nil || p.program == nil || vm == nil {
		return nil, ErrInvalidInput
	}

	// Dynamic eval/Function parsing must not load authored source-map URLs from
	// disk or a caller's loader. Existing precompiled library programs are reused.
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(p.limits.MaxStackDepth)

	for _, name := range toolHostNames {
		value := vm.Get(name)
		if value == nil {
			value = goja.Undefined()
		}
		enumerable := goja.FLAG_FALSE
		switch name {
		case "params", "context", "response", "cfg", "labels", "annotations":
			// Preserve the shared input binder's already-frozen properties;
			// capabilities such as fetch retain their non-enumerable contract.
			enumerable = goja.FLAG_NOT_SET
		}
		if err := vm.GlobalObject().DefineDataProperty(
			name,
			value,
			goja.FLAG_FALSE, // writeable
			goja.FLAG_FALSE, // configurable
			enumerable,
		); err != nil {
			return nil, ErrInvalidInput
		}
	}

	if err := p.library.Run(vm); err != nil {
		return nil, runtimeEvaluationError(err)
	}

	if _, err := vm.RunProgram(p.program); err != nil {
		return nil, runtimeEvaluationError(err)
	}

	callable, ok := goja.AssertFunction(vm.Get("execute"))
	if !ok {
		return nil, ErrEvaluation
	}

	return callable, nil
}

// Validate checks top-level initialization in a fresh runtime under the Tool's
// timeout and stack policy. It never supplies host capabilities, connection
// values, or execute arguments, and never invokes the returned entry point.
// Unhandled initialization rejections fail just as they do during evaluation;
// helpers that need captured input must read it when called by execute.
// Caller cancellation retains its context error; policy expiry or stack
// exhaustion returns ErrLimitExceeded. Any interrupt watcher is joined before
// returning, and no runtime or pending authored state is reused by invocation.
func (p *ToolProgram) Validate(ctx context.Context) error {
	if p == nil || p.program == nil || ctx == nil {
		return ErrInvalidInput
	}
	return withRuntime(ctx, p.limits, func(_ context.Context, vm *goja.Runtime) error {
		tracker := newPromiseTracker(vm)
		defer tracker.close()
		if _, err := p.Initialize(vm); err != nil {
			return err
		}
		return tracker.checkRejections()
	})
}
