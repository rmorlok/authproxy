// Package runtime provides a bounded Goja lifecycle, native JSON conversion,
// and optional HTTP and authored logging bindings for trusted host adapters.
// It has no Tool, connection lookup, authorization, or HTTP-client selection policy.
package runtime

import (
	"errors"
	"time"
)

// These errors classify failures without disclosing authored code, input data,
// thrown values, or JavaScript stacks to callers.
var (
	ErrInvalidSource = errors.New("invalid JavaScript source")
	ErrInvalidInput  = errors.New("invalid JavaScript input")
	ErrInvalidOutput = errors.New("JavaScript result is not a supported JSON value")
	ErrLimitExceeded = errors.New("JavaScript execution limit exceeded")
	ErrEvaluation    = errors.New("JavaScript execution failed")
)

// Limits contains resolved positive caps for compilation and execution. Callers
// apply their own policy ceilings; this package supplies no unlimited defaults.
// These limits bound host work and execution time, not the VM's heap usage.
type Limits struct {
	// MaxSourceBytes bounds authored UTF-8 source before parsing, including any
	// helper libraries. CompileExpression and CompileAsync enforce it; adapters
	// compiling other program types apply the same source allowance.
	MaxSourceBytes int

	// MaxInputBytes bounds combined serialized input. The input adapter shares
	// this allowance across its documents before constructing native VM values.
	MaxInputBytes int

	// MaxOutputBytes bounds emitted JSON, including punctuation and escaping.
	// It also bounds each authored log record's combined message/fields encoding.
	// A domain adapter's final result envelope needs a separate byte allowance.
	MaxOutputBytes int

	// MaxDepth bounds JSON value nesting; the root is depth zero.
	MaxDepth int

	// MaxNodes bounds JSON values and object property inspections, separately
	// for combined input, output, and each authored log record. It does not count
	// JavaScript instructions.
	MaxNodes int

	// MaxStackDepth bounds JavaScript function call depth, including recursion.
	MaxStackDepth int

	// Timeout covers the entire Run or EvaluateJSON callback and result
	// conversion. Policy expiry is ErrLimitExceeded; caller cancellation keeps
	// its context error. Native Goja built-ins cannot be preempted immediately.
	Timeout time.Duration

	// MaxWorkUnits bounds cumulative managed native operations, inspected
	// bytes/code units, and visited values across the execution's conversions,
	// pure utilities, HTTP, and authored logging. It does not meter JavaScript
	// instructions.
	MaxWorkUnits int

	// MaxAllocationUnits bounds cumulative managed buffer bytes, string UTF-16
	// code units, and native value/tuple slots, including intermediate copies.
	// These logical units bound storage by fixed multiples, not total VM heap
	// bytes. Initialization, execution, and conversion share one allowance.
	MaxAllocationUnits int
}

// Validate rejects missing required bounds before VM allocation.
// Adapters also use it when compiling programs that capture these limits.
func (l Limits) Validate() error {
	if l.MaxSourceBytes <= 0 ||
		l.MaxInputBytes <= 0 ||
		l.MaxOutputBytes <= 0 ||
		l.MaxDepth <= 0 ||
		l.MaxNodes <= 0 ||
		l.MaxStackDepth <= 0 ||
		l.Timeout <= 0 ||
		l.MaxWorkUnits <= 0 ||
		l.MaxAllocationUnits <= 0 {
		return ErrInvalidInput
	}
	return nil
}
