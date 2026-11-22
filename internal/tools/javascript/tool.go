package javascript

import (
	"context"

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
	program *apjsruntime.ExtendedAsyncProgram[*Result]

	// limits is the same copied policy used to bind Tool inputs and HTTP hosts.
	limits Limits
}

// CompileTool prepares an async execute declaration and connector helpers using
// the Tool host binding policy. It executes no authored code. Source limits,
// declaration conflicts, and safe error classifications come from the shared
// compiler; invocation still supplies params and captured context positionally.
func CompileTool(source string, library *apjs.Library, limits Limits) (*ToolProgram, error) {
	program, err := apjsruntime.CompileExtendedAsync(
		source,
		library,
		toolHostBindings,
		limits,
		toolResultExtension(),
	)
	if err != nil {
		return nil, err
	}

	return &ToolProgram{
		program: program,
		limits:  limits,
	}, nil
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
