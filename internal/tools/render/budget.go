// Package render builds tool request values from a private invocation snapshot.
// It performs no I/O and does not change connector or OAuth template behavior.
package render

import (
	"context"
	"errors"
)

// Limits bounds a single context snapshot, template compilation, or render.
// Callers must supply positive limits from their execution policy. MaxBytes
// bounds source and output sizes; MaxDepth bounds nesting; MaxSteps bounds
// visited values and template operations, including repeated section bodies.
type Limits struct {
	MaxBytes int
	MaxDepth int
	MaxSteps int
}

// validate rejects disabled limits rather than silently allowing unbounded work.
func (l Limits) validate() error {
	if l.MaxBytes <= 0 || l.MaxDepth <= 0 || l.MaxSteps <= 0 {
		return errors.New("render limits must all be positive")
	}
	return nil
}

// budget accounts for one operation. It is never shared between goroutines.
type budget struct {
	limit Limits
	ctx   context.Context
	steps int
	bytes int
}

// newBudget starts fresh accounting for a validated set of limits.
func newBudget(ctx context.Context, limits Limits) *budget {
	return &budget{limit: limits, ctx: ctx}
}

// step checks cancellation and charges one visited value or template operation.
func (b *budget) step() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.steps >= b.limit.MaxSteps {
		return errors.New("render step limit exceeded")
	}
	b.steps++
	return nil
}

// addBytes checks before addition so even unusually large limits cannot overflow.
func (b *budget) addBytes(n int) error {
	if n > b.limit.MaxBytes-b.bytes {
		return errors.New("render byte limit exceeded")
	}
	b.bytes += n
	return nil
}
