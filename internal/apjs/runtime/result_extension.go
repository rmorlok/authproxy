package runtime

import (
	"context"

	"github.com/dop251/goja"
)

// ResultExtension defines pure helpers and domain result interpretation for an
// async program. It adds no I/O capabilities. The compiler copies its binding
// policy, and Install creates separate branding and conversion state in each VM.
// T must contain only detached Go data, never VM values, callbacks, or hosts.
type ResultExtension[T any] struct {
	// Bindings declares globals installed by this extension. Names are reserved
	// during compilation and frozen before authored initialization, just like
	// other host bindings. Duplicates across both binding lists are invalid.
	Bindings []HostBinding

	// Install runs on the VM owner before authored initialization during both
	// validation and evaluation. It must be pure, obey ctx, and retain VM state
	// only in the returned interpreter and installed helpers. Each call creates
	// fresh branding state. The function itself must support concurrent calls.
	// It returns only safe errors; sanitize authored errors with EvaluationError.
	Install func(context.Context, *goja.Runtime) (ResultInterpreter[T], error)
}

// ResultInterpreter converts settled results on their VM's owner, after host
// cleanup and before the runtime deadline ends. Use the supplied encoder for
// every authored payload field; all fields share one output allowance. Callbacks
// must honor ctx, avoid unchecked Export/coercion/getters, and return only
// detached data or safe errors. They must not retain the encoder or VM values.
// Caller cancellation, policy expiration, and encoder failures discard T.
type ResultInterpreter[T any] struct {
	// Fulfilled is required. It recognizes branded successes and defines the
	// domain's ordinary-value conversion. Branding must use private identity,
	// not user-visible property names that ordinary objects could imitate.
	Fulfilled func(context.Context, goja.Value, *ResultEncoder) (T, error)

	// Rejected is optional. It may interpret only an explicitly branded root
	// rejection. recognized=false discards both the value and error, returning
	// ErrEvaluation unless conversion itself failed. Unrelated unhandled
	// rejections prevent this callback from running; ordinary thrown values
	// must remain unrecognized and private.
	Rejected func(context.Context, goja.Value, *ResultEncoder) (value T, recognized bool, err error)
}

// ExtendedAsyncProgram pairs a compiled async program with a trusted result
// extension. It retains no invocation data and supports concurrent evaluation.
// The ordinary AsyncProgram API continues to return detached JSON unchanged.
type ExtendedAsyncProgram[T any] struct {
	// program owns the shared compilation, lifecycle, and copied binding policy.
	program *AsyncProgram

	// install creates the interpreter and pure helpers independently in each VM.
	install func(context.Context, *goja.Runtime) (ResultInterpreter[T], error)
}

// CompileExtendedAsync compiles the same async execute contract as CompileAsync,
// adding the extension's reserved bindings. Compilation calls no installer or
// authored code. Validate and Evaluate install the same pure helpers before
// initialization; neither the extension nor its helpers may perform I/O there.
func CompileExtendedAsync[T any](
	source string,
	library CompiledLibrary,
	bindings []HostBinding,
	limits Limits,
	extension ResultExtension[T],
) (*ExtendedAsyncProgram[T], error) {
	if extension.Install == nil {
		return nil, ErrInvalidInput
	}
	combined := make([]HostBinding, 0, len(bindings)+len(extension.Bindings))
	combined = append(combined, bindings...)
	combined = append(combined, extension.Bindings...)
	program, err := CompileAsync(source, library, combined, limits)
	if err != nil {
		return nil, err
	}
	return &ExtendedAsyncProgram[T]{program: program, install: extension.Install}, nil
}

// Validate installs pure helpers and runs bounded initialization without inputs,
// HTTP hosts, result interpretation, or calling execute. Installer errors and
// unhandled initialization rejections fail validation just as in evaluation.
func (p *ExtendedAsyncProgram[T]) Validate(ctx context.Context) error {
	if p == nil || p.program == nil || p.install == nil {
		return ErrInvalidInput
	}
	return p.program.validate(ctx, func(runCtx context.Context, vm *goja.Runtime) error {
		interpreter, err := p.install(runCtx, vm)
		if err != nil {
			return err
		}
		if interpreter.Fulfilled == nil {
			return ErrInvalidInput
		}
		return nil
	})
}

// Evaluate runs execute with prepared native inputs and interprets its settled
// value before disposing of the VM. Preparation, helpers, host work, cleanup,
// and interpretation share one deadline. Any failure returns the zero value of
// T, including cancellation after a converter produced a detached result.
func (p *ExtendedAsyncProgram[T]) Evaluate(
	ctx context.Context,
	prepare func(context.Context, *goja.Runtime, *AsyncInvocation) error,
) (T, error) {
	if p == nil {
		var zero T
		return zero, ErrInvalidInput
	}
	return evaluateAsync(ctx, p.program, prepare, p.install)
}

// evaluateAsync is the common bounded result path for ordinary JSON and domain
// extensions. The encoder captures pristine intrinsics before any installer or
// preparation, and its sticky failure cannot be hidden by a result callback.
func evaluateAsync[T any](
	ctx context.Context,
	p *AsyncProgram,
	prepare func(context.Context, *goja.Runtime, *AsyncInvocation) error,
	install func(context.Context, *goja.Runtime) (ResultInterpreter[T], error),
) (T, error) {
	var result, zero T
	if p == nil || p.program == nil || ctx == nil || prepare == nil || install == nil {
		return zero, ErrInvalidInput
	}
	err := Run(ctx, p.limits, func(runCtx context.Context, vm *goja.Runtime) error {
		encoder := newResultEncoder(runCtx, vm, p.limits)
		var interpreter ResultInterpreter[T]
		return p.evaluate(runCtx, vm, prepare, func() error {
			var err error
			interpreter, err = install(runCtx, vm)
			if err != nil {
				return err
			}
			if interpreter.Fulfilled == nil {
				return ErrInvalidInput
			}
			return nil
		}, func(root *goja.Promise) error {
			var err error
			switch root.State() {
			case goja.PromiseStateFulfilled:
				result, err = interpreter.Fulfilled(runCtx, root.Result(), encoder)
			case goja.PromiseStateRejected:
				if interpreter.Rejected == nil {
					return ErrEvaluation
				}
				var recognized bool
				result, recognized, err = interpreter.Rejected(runCtx, root.Result(), encoder)
				if !recognized {
					err = ErrEvaluation
				}
			default:
				return ErrEvaluation
			}
			if encoder.err != nil {
				return encoder.err
			}
			return err
		})
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}
