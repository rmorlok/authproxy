package runtime

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// CompiledLibrary supplies trusted, already-compiled helpers without coupling
// the runtime to a connector implementation. Source must describe exactly the
// immutable code Run executes. Both methods may be called concurrently, with
// each Run receiving a different VM. A nil interface means no helper library.
// Implementations passed as typed nil pointers must make their methods nil-safe.
type CompiledLibrary interface {
	// Source returns authored UTF-8 for combined byte accounting and declaration
	// checks. Merely inspecting source must not execute the helper program.
	Source() string

	// Run initializes helpers in the caller-owned VM without adding Go hosts or
	// I/O capabilities. It obeys the VM's existing stack/interrupt policy and
	// retains no VM values. Returned diagnostics are sanitized by this package.
	Run(*goja.Runtime) error
}

// asyncFunctionPrototype reads the runtime's AsyncFunction.prototype intrinsic.
// Initialize runs it before authored code, when the intrinsic cannot be disturbed.
var asyncFunctionPrototype = goja.MustCompile(
	"async-function-prototype.js",
	`Object.getPrototypeOf(async function () {})`,
	true, // strict
)

// AsyncProgram owns a compiled async entry point and its optional compiled
// helper library. It retains no runtime or invocation values and can serve
// concurrent evaluations, each in a fresh VM. Compilation runs no authored code;
// validation runs initialization without I/O capabilities and does not invoke
// execute. Evaluate owns the complete bounded invocation and JSON conversion.
type AsyncProgram struct {
	program  *goja.Program   // Strict async script, including its execute declaration.
	library  CompiledLibrary // Trusted immutable helpers, preserving their compilation mode.
	limits   Limits          // Captured source, execution, and input/output policy.
	bindings []HostBinding   // Private host-name and property policy, copied at compilation.
}

// CompileAsync compiles a script with exactly one top-level async, non-generator
// function declaration named execute. Parameter names and arity are left to the
// author; invocation will supply arguments and captured context positionally.
// Other helpers and top-level initialization are allowed. This function never
// runs authored code; Validate checks initialization separately without I/O.
//
// The combined entry-point and library source must fit MaxSourceBytes. Global host
// declarations are rejected, including uninitialized and destructured bindings;
// ordinary function-local or block-local declarations remain valid. Legacy
// unbraced conditional declarations of reserved names are rejected anywhere in
// a library because they can reach the global object. A helper library cannot
// declare execute because that entry point belongs to the main program. The
// caller supplies domain-specific host bindings. The shared fetch name is always
// reserved, even when direct HTTP is disabled; an explicit fetch descriptor may
// repeat that default but cannot preserve enumerability. Malformed binding
// options return ErrInvalidInput.
func CompileAsync(
	source string,
	library CompiledLibrary,
	bindings []HostBinding,
	limits Limits,
) (*AsyncProgram, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}

	bindings, err := copyHostBindings(bindings)
	if err != nil {
		return nil, err
	}

	// Reserve the optional built-in capability before source and library checks.
	// Existing adapters may already include it in their host policy.
	hasFetch := false
	for _, binding := range bindings {
		if binding.Name == "fetch" {
			if binding.PreserveEnumerable {
				return nil, ErrInvalidInput
			}
			hasFetch = true
		}
	}
	if !hasFetch {
		bindings = append(bindings, HostBinding{Name: "fetch"})
	}

	librarySource := ""
	if library != nil {
		librarySource = library.Source()
	}
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
		"async-program.js",
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

	if err := checkHostBindings(
		source,
		bindings,
		true,  // strict
		false, // reserveExecute
	); err != nil {
		return nil, err
	}

	if err := checkHostBindings(
		librarySource,
		bindings,
		false, // strict
		true,  // reserveExecute
	); err != nil {
		return nil, err
	}

	return &AsyncProgram{
		program:  program,
		library:  library,
		limits:   limits,
		bindings: bindings,
	}, nil
}

// Initialize runs helper initialization and then the async script in vm,
// returning its callable execute binding without invoking it. Compilation checks
// the async declaration; because top-level code can reassign that binding,
// initialization checks that execute is still an async function afterward. This
// catches replacement by a synchronous function or noncallable value; it is a
// contract check, not a sandbox, so code that deliberately spoofs the prototype
// is not detected. The caller must own a fresh runtime on one goroutine, install
// copied inputs and gated host capabilities first, and keep cancellation/time
// limits active through initialization and subsequent calls. Hosts must reject I/O
// until initialization and rejection checks succeed. Only Goja's Interrupt may
// run on another goroutine.
// The returned callable belongs exclusively to vm and must not outlive it.
//
// Host bindings are made nonwritable and nonconfigurable before authored code
// runs; missing capabilities remain undefined. Stack policy is applied here.
// Top-level errors and noncallable entry points return fixed classifications,
// never source, thrown values, or library diagnostics. Discard vm on failure.
func (p *AsyncProgram) Initialize(vm *goja.Runtime) (goja.Callable, error) {
	if p == nil || p.program == nil || vm == nil {
		return nil, ErrInvalidInput
	}

	// Dynamic eval/Function parsing must not load authored source-map URLs from
	// disk or a caller's loader. Existing precompiled library programs are reused.
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(p.limits.MaxStackDepth)

	asyncPrototype, err := vm.RunProgram(asyncFunctionPrototype)
	if err != nil {
		return nil, ErrInvalidInput
	}
	getPrototype, _ := goja.AssertFunction(vm.Get("Object").ToObject(vm).Get("getPrototypeOf"))

	for _, binding := range p.bindings {
		value := vm.Get(binding.Name)
		if value == nil {
			value = goja.Undefined()
		}
		enumerable := goja.FLAG_FALSE
		if binding.PreserveEnumerable {
			// Already-frozen JSON aliases keep their existing visibility.
			enumerable = goja.FLAG_NOT_SET
		}
		if err := vm.GlobalObject().DefineDataProperty(
			binding.Name,
			value,
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			enumerable,
		); err != nil {
			return nil, ErrInvalidInput
		}
	}

	if p.library != nil {
		if err := p.library.Run(vm); err != nil {
			return nil, EvaluationError(err)
		}
	}

	if _, err := vm.RunProgram(p.program); err != nil {
		return nil, EvaluationError(err)
	}

	entry := vm.Get("execute")
	callable, ok := goja.AssertFunction(entry)
	if !ok {
		return nil, ErrEvaluation
	}

	// Calling the captured intrinsic also drains jobs created by a Proxy's
	// prototype trap. Direct Object.Prototype access would leave those jobs
	// queued until execute, after an invocation host has enabled network I/O.
	prototype, err := getPrototype(goja.Undefined(), entry)
	if err != nil {
		return nil, EvaluationError(err)
	}
	if prototype != asyncPrototype {
		return nil, ErrEvaluation
	}

	return callable, nil
}

// Validate checks top-level initialization in a fresh runtime under the program's
// timeout and stack policy. It never supplies host capabilities, connection
// values, or execute arguments, and never invokes the returned entry point.
// Unhandled initialization rejections fail just as they do during evaluation;
// this also catches unavailable capabilities used by async top-level work.
// Helpers that need invocation input must read it when called by execute.
// Caller cancellation retains its context error; policy expiry or stack
// exhaustion returns ErrLimitExceeded. Any interrupt watcher is joined before
// returning, and no runtime or pending authored state is reused by invocation.
func (p *AsyncProgram) Validate(ctx context.Context) error {
	return p.validate(ctx, nil)
}

// validate checks authored initialization in a fresh bounded VM without calling
// execute. The optional install callback binds the extension's pure JavaScript
// helpers and creates their per-VM branding and result interpreter, so top-level
// source and library code can use the same helpers as a real invocation. The
// interpreter is checked for validity but is not called during validation.
//
// install runs on the VM owner with rejection tracking already active and must
// honor its execution context, perform no I/O, and return only safe errors. A nil
// callback installs no extension. No invocation inputs or HTTP hosts are supplied;
// installer errors and unhandled initialization rejections fail validation.
func (p *AsyncProgram) validate(
	ctx context.Context,
	install func(context.Context, *goja.Runtime) error,
) error {
	if p == nil || p.program == nil || ctx == nil {
		return ErrInvalidInput
	}

	return Run(ctx, p.limits, func(runCtx context.Context, vm *goja.Runtime) error {
		tracker := NewPromiseTracker(vm)
		defer tracker.Close()

		if install != nil {
			if err := install(runCtx, vm); err != nil {
				return err
			}
		}

		if _, err := p.Initialize(vm); err != nil {
			return err
		}

		return tracker.CheckRejections()
	})
}
