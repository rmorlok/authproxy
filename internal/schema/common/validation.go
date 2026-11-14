package common

import (
	"context"
	"fmt"
)

type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationContext carries a field path and optional caller cancellation
// through nested schema validation. Child paths preserve the same context.
type ValidationContext struct {
	Path string
	// Ctx is the request/task context for validation that executes authored
	// code. Nil supports synchronous configuration validation; execution still
	// uses finite policy limits even when there is no caller deadline.
	Ctx context.Context
}

// Context returns the caller context, or a background context for existing
// synchronous validators. It accepts a nil receiver for optional root contexts.
func (c *ValidationContext) Context() context.Context {
	if c != nil && c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// PushField returns a child field path without changing this validation context.
func (c *ValidationContext) PushField(field string) *ValidationContext {
	child := *c
	if len(c.Path) == 0 {
		child.Path = field
	} else {
		child.Path = c.Path + "." + field
	}
	return &child
}

// PushIndex returns an array element path while preserving caller cancellation.
func (c *ValidationContext) PushIndex(i int) *ValidationContext {
	child := *c
	child.Path = fmt.Sprintf("%s[%d]", c.Path, i)
	return &child
}

func (c *ValidationContext) NewError(message string) *ValidationError {
	return &ValidationError{Path: c.Path, Message: message}
}

func (c *ValidationContext) NewErrorf(format string, args ...interface{}) *ValidationError {
	return c.NewError(fmt.Sprintf(format, args...))
}

func (c *ValidationContext) NewErrorForField(field, message string) *ValidationError {
	return c.PushField(field).NewError(message)
}

func (c *ValidationContext) NewErrorfForField(field, format string, args ...interface{}) *ValidationError {
	return c.PushField(field).NewErrorf(format, args...)
}
