package javascript

import (
	"context"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// toolHostBindings declares the Tool host surface. Missing capabilities remain
// undefined; reserving a name does not enable it. Input aliases preserve their
// existing enumerability, while capability names are non-enumerable.
var toolHostBindings = []apjsruntime.HostBinding{
	{Name: "fetch"},
	{Name: "URL"},
	{Name: "URLSearchParams"},
	{Name: "TextEncoder"},
	{Name: "TextDecoder"},
	{Name: "atob"},
	{Name: "btoa"},
	{Name: "console"},
	{Name: "ToolError"},
	{Name: "toolResult"},
	{Name: "params", PreserveEnumerable: true},
	{Name: "context", PreserveEnumerable: true},
	{Name: "cfg", PreserveEnumerable: true},
	{Name: "labels", PreserveEnumerable: true},
	{Name: "annotations", PreserveEnumerable: true},
	{Name: "response", PreserveEnumerable: true},
	{Name: "data"},
}

// ToolProgram adapts a shared async program to Tool inputs and optional connector
// helpers. It retains no invocation state and may serve concurrent evaluations.
type ToolProgram struct {
	// program owns compilation, validation, and bounded async execution.
	program *apjsruntime.AsyncProgram

	// limits is the same copied policy used to bind Tool inputs and HTTP hosts.
	limits Limits
}

// CompileTool prepares an async execute declaration and connector helpers using
// the Tool host binding policy. It executes no authored code. Source limits,
// declaration conflicts, and safe error classifications come from the shared
// compiler; invocation still supplies params and captured context positionally.
func CompileTool(source string, library *apjs.Library, limits Limits) (*ToolProgram, error) {
	program, err := apjsruntime.CompileAsync(source, library, toolHostBindings, limits)
	if err != nil {
		return nil, err
	}
	return &ToolProgram{program: program, limits: limits}, nil
}

// Initialize protects Tool globals, initializes helpers and source, and returns
// the checked async execute binding. The caller owns a fresh VM, keeps its
// deadline active, and gates I/O until initialization and its rejection checks
// succeed. The returned callable belongs to that VM and must not outlive it.
func (p *ToolProgram) Initialize(vm *goja.Runtime) (goja.Callable, error) {
	if p == nil || p.program == nil {
		return nil, apjsruntime.ErrInvalidInput
	}
	return p.program.Initialize(vm)
}

// Validate runs bounded initialization with no inputs or I/O and without calling
// execute. Shared validation rejects unhandled initialization promises and keeps
// caller cancellation distinct from policy expiry and authored failures.
func (p *ToolProgram) Validate(ctx context.Context) error {
	if p == nil || p.program == nil {
		return apjsruntime.ErrInvalidInput
	}
	return p.program.Validate(ctx)
}
