// Package javascript evaluates trusted Tool expressions with native JavaScript
// copies of captured invocation data. It exposes no I/O or application handles.
package javascript

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// These errors classify failures without disclosing authored code, connection
// configuration, thrown values, or JavaScript stacks to invocation callers.
var (
	ErrInvalidSource = errors.New("invalid JavaScript expression")
	ErrInvalidInput  = errors.New("invalid JavaScript input")
	ErrInvalidOutput = errors.New("JavaScript result is not a supported JSON value")
	ErrLimitExceeded = errors.New("JavaScript execution limit exceeded")
	ErrEvaluation    = errors.New("JavaScript expression failed")
)

// Limits contains resolved positive caps for one compiled expression. Callers
// apply operator ceilings before compilation. These caps bound host processing
// and interrupt JavaScript execution; they are not a hard memory quota.
type Limits struct {
	// MaxSourceBytes bounds the authored UTF-8 expression before parsing.
	MaxSourceBytes int
	// MaxInputBytes bounds serialized snapshot and response JSON together.
	MaxInputBytes int
	// MaxOutputBytes bounds the emitted JSON, including quotes and escaping.
	// A later result envelope needs its own serialized byte allowance.
	MaxOutputBytes int
	// MaxDepth bounds JSON value nesting; the root is depth zero.
	MaxDepth int
	// MaxNodes bounds JSON values and object property inspections, separately
	// for the combined input and output. It does not count JavaScript bytecode.
	MaxNodes int
	// MaxStackDepth bounds JavaScript function call depth, including recursion.
	MaxStackDepth int
	// Timeout covers input binding, expression execution, and result conversion.
	// A shorter caller deadline wins. Native Goja built-ins cannot be preempted;
	// cancellation is checked again before any result is returned.
	Timeout time.Duration
}

// validate disallows omitted or unlimited policy values.
func (l Limits) validate() error {
	if l.MaxSourceBytes <= 0 || l.MaxInputBytes <= 0 || l.MaxOutputBytes <= 0 ||
		l.MaxDepth <= 0 || l.MaxNodes <= 0 || l.MaxStackDepth <= 0 || l.Timeout <= 0 {
		return ErrInvalidInput
	}
	return nil
}

// Input supplies a captured safe invocation snapshot and optional response data.
// Neither field exposes a connection handle, credentials, or arbitrary Go objects.
// The caller must not modify Response bytes during Evaluate.
type Input struct {
	// Context contains validated params and copied configuration, metadata, and
	// identities. Each evaluation receives a fresh native JavaScript copy.
	Context *render.Context
	// Response is an object of caller-prepared response fields. Nil means {}.
	// The HTTP executor will prepare status, headers, body, and mediaType; this
	// primitive does not read streams or interpret HTTP content types.
	Response common.RawJSON
}

// Expression is an immutable, reusable pure expression program. Evaluation
// always creates a fresh VM, so callers can reuse it concurrently. It has no
// asynchronous host, connector helper library, or network capability.
type Expression struct {
	program *goja.Program
	limits  Limits
}

// CompileExpression parses exactly one strict JavaScript expression without
// executing authored code. Function-call expressions and IIFEs are allowed;
// statements cannot escape the expression wrapper. Syntax failures are safe
// classifications rather than parser messages containing source text.
func CompileExpression(source string, limits Limits) (*Expression, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if len(source) > limits.MaxSourceBytes {
		return nil, ErrLimitExceeded
	}
	if !utf8.ValidString(source) || strings.TrimSpace(source) == "" {
		return nil, ErrInvalidSource
	}
	parsed, err := parser.ParseFile(nil, "tool-expression.js", "(\n"+source+"\n)", 0)
	if err != nil || len(parsed.Body) != 1 {
		return nil, ErrInvalidSource
	}
	if _, ok := parsed.Body[0].(*ast.ExpressionStatement); !ok {
		return nil, ErrInvalidSource
	}
	program, err := goja.CompileAST(parsed, true)
	if err != nil {
		return nil, ErrInvalidSource
	}
	return &Expression{program: program, limits: limits}, nil
}

// Evaluate returns a detached JSON value or an error with no partial output.
// Native objects can be mutated during this call without changing the captured
// snapshot or another invocation. The result uses JavaScript Number semantics;
// this is not a precision-preserving raw JSON passthrough.
//
// Cancellation and the configured timeout remain active through conversion.
// Only the interrupt watcher accesses the VM from another goroutine, through
// Goja's thread-safe Interrupt method. Thrown values and stacks are never exposed.
func (e *Expression) Evaluate(ctx context.Context, input Input) (result common.RawJSON, err error) {
	if e == nil || e.program == nil || ctx == nil {
		return nil, ErrInvalidInput
	}
	runCtx, cancel := context.WithTimeout(ctx, e.limits.Timeout)
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return nil, err
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(e.limits.MaxStackDepth)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-runCtx.Done():
			vm.Interrupt(runCtx.Err())
		case <-stop:
		}
	}()
	defer func() {
		// Join before discarding the VM, so no late interrupt can outlive this
		// invocation. Fresh VMs avoid any need to clear or reuse interruption.
		close(stop)
		<-stopped
		if recovered := recover(); recovered != nil {
			switch recovered.(type) {
			case *goja.InterruptedError:
				result, err = nil, ErrEvaluation
			case *goja.StackOverflowError:
				result, err = nil, ErrLimitExceeded
			default:
				panic(recovered)
			}
		}
		if contextErr := runCtx.Err(); contextErr != nil {
			result, err = nil, contextErr
		}
	}()

	// Capture conversion intrinsics before authored code can replace globals or
	// built-in methods. Input binding also runs before any author expression.
	encoder := newOutputEncoder(vm, e.limits)
	if err := bindInput(runCtx, vm, input, e.limits); err != nil {
		return nil, err
	}
	value, err := vm.RunProgram(e.program)
	if err != nil {
		var stackErr *goja.StackOverflowError
		if errors.As(err, &stackErr) {
			return nil, ErrLimitExceeded
		}
		return nil, ErrEvaluation
	}
	return encoder.encode(runCtx, value)
}
