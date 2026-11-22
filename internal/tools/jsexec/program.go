// Package jsexec prepares JavaScript Tool definitions for admitted execution.
// It combines captured schemas, the selected connector library, and effective
// policy without granting connection access or exposing a public invocation API.
package jsexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// Program owns the compiled script, schemas, and policy for one captured Tool
// revision and selected connector library. It retains no invocation or VM data
// and supports concurrent initialization checks, each in a fresh bounded VM.
// A program must not be reused for another revision, library, or policy.
type Program struct {
	script  *javascript.ToolProgram // Tool-only helpers and the shared async runtime.
	schemas *validation.Program     // Captured input and optional successful-output schemas.
	limits  Limits                  // Copied operator policy lowered by authored caps.
}

// Compile prepares exactly the javascript executor without running any authored
// code or making requests. The caller selects the captured definition and its
// connector library, validates the full resource, and supplies finite operator
// policy. Optional authored limits can only lower those ceilings. No caller-owned
// definition, policy, or library wrapper is retained after compilation.
//
// The library must already have passed source admission before its own parsing;
// Compile checks its retained source together with the Tool source. Its immutable
// compiled code is reused, preserving existing connector JavaScript semantics.
// Validate separately checks initialization under the effective runtime policy.
//
// Schema compilation is synchronous and still needs the separately tracked
// evaluation/compilation work bound before public invocation can be enabled.
// Compiler errors are for trusted callers, not model-facing ToolResult messages.
func Compile(
	definition toolschema.ToolDefinition,
	library *apjs.Library,
	limits Limits,
) (*Program, error) {
	if definition.Javascript == nil || definition.ProxyHTTP != nil {
		return nil, errors.New("JavaScript Tool requires only the javascript executor")
	}

	limits, err := ResolveLimits(limits, definition.Limits)
	if err != nil {
		return nil, err
	}
	if err := limits.validateHostLimits(); err != nil {
		return nil, err
	}

	// The library's compiled code and source are immutable. Copy its wrapper so
	// replacing the caller's Library value cannot change checked code later.
	var capturedLibrary *apjs.Library
	if library != nil {
		copy := *library
		capturedLibrary = &copy
	}

	script, err := javascript.CompileTool(*definition.Javascript, capturedLibrary, limits.Runtime)
	if err != nil {
		return nil, fmt.Errorf("JavaScript Tool source: %w", err)
	}

	schemas, err := validation.Compile(definition.InputSchema, definition.OutputSchema, limits.Validation)
	if err != nil {
		return nil, fmt.Errorf("JavaScript Tool schemas: %w", err)
	}
	return &Program{script: script, schemas: schemas, limits: limits}, nil
}

// Validate checks connector and Tool initialization without invoking execute or
// supplying arguments, connection configuration, HTTP, or authored logging.
// The shared runtime installs pure utilities and Tool result/error helpers, then
// applies the effective time, stack, and cumulative work/allocation limits.
// Cancellation remains a caller context error; policy exhaustion and authored
// failures retain safe runtime categories without source or thrown values.
// This check validates initialization, not the result of every possible call.
func (p *Program) Validate(ctx context.Context) error {
	if p == nil || p.script == nil || p.schemas == nil {
		return apjsruntime.ErrInvalidInput
	}
	return p.script.Validate(ctx)
}
