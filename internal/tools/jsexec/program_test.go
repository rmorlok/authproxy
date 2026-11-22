package jsexec

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// compiledDefinition supplies object arguments and a successful structured result
// schema so compilation tests exercise both captured validators.
func compiledDefinition(source string) toolschema.ToolDefinition {
	return toolschema.ToolDefinition{
		Javascript:   util.ToPtr(source),
		InputSchema:  common.RawJSON(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		OutputSchema: common.RawJSON(`{"type":"object","properties":{"saved":{"type":"boolean"}},"required":["saved"]}`),
	}
}

// compiledLibrary parses helper source without running it, letting tests prove
// that the composite compiler also leaves authored initialization to Validate.
func compiledLibrary(t *testing.T, source string) *apjs.Library {
	t.Helper()
	library, err := apjs.CompileLibrary(source)
	require.NoError(t, err)
	return library
}

// TestCompileRejectsInvalidDefinitions checks executor selection, schema
// contracts, and Tool source policy before publishing a reusable program.
func TestCompileRejectsInvalidDefinitions(t *testing.T) {
	for name, change := range map[string]func(*toolschema.ToolDefinition){
		"missing executor":  func(d *toolschema.ToolDefinition) { d.Javascript = nil },
		"two executors":     func(d *toolschema.ToolDefinition) { d.ProxyHTTP = &toolschema.ProxyHTTP{} },
		"HTTP executor":     func(d *toolschema.ToolDefinition) { d.Javascript = nil; d.ProxyHTTP = &toolschema.ProxyHTTP{} },
		"empty source":      func(d *toolschema.ToolDefinition) { *d.Javascript = " " },
		"syntax":            func(d *toolschema.ToolDefinition) { *d.Javascript = "private invalid source {" },
		"synchronous entry": func(d *toolschema.ToolDefinition) { *d.Javascript = "function execute() {}" },
		"host declaration":  func(d *toolschema.ToolDefinition) { *d.Javascript = "const ToolError = 1; async function execute() {}" },
		"input schema":      func(d *toolschema.ToolDefinition) { d.InputSchema = common.RawJSON(`true`) },
		"output schema":     func(d *toolschema.ToolDefinition) { d.OutputSchema = common.RawJSON(`null`) },
		"external schema": func(d *toolschema.ToolDefinition) {
			d.OutputSchema = common.RawJSON(`{"$ref":"https://example.invalid/private-schema"}`)
		},
		"authored limit": func(d *toolschema.ToolDefinition) { d.Limits = &toolschema.ExecutionLimits{MaxRequests: util.ToPtr(0)} },
	} {
		t.Run(name, func(t *testing.T) {
			// A malformed component returns no partially compiled executor.
			definition := compiledDefinition("async function execute() { return {saved: true}; }")
			change(&definition)
			program, err := Compile(definition, nil, policyLimits())
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// TestCompileRequiresCompletePolicy checks policy used by later execution as
// well as compilation; authored caps cannot supply missing operator ceilings.
func TestCompileRequiresCompletePolicy(t *testing.T) {
	for name, change := range map[string]func(*Limits){
		"snapshot bytes":      func(l *Limits) { l.Render.MaxBytes = 0 },
		"snapshot depth":      func(l *Limits) { l.Render.MaxDepth = -1 },
		"snapshot steps":      func(l *Limits) { l.Render.MaxSteps = 0 },
		"argument bytes":      func(l *Limits) { l.Validation.MaxInputBytes = 0 },
		"schema output bytes": func(l *Limits) { l.Validation.MaxOutputBytes = 0 },
		"schema depth":        func(l *Limits) { l.Validation.MaxDepth = 0 },
		"schema values":       func(l *Limits) { l.Validation.MaxValues = 0 },
		"number digits":       func(l *Limits) { l.Validation.MaxNumberDigits = 4097 },
		"runtime policy":      func(l *Limits) { l.Runtime.MaxAllocationUnits = 0 },
		"runtime source":      func(l *Limits) { l.Runtime.MaxSourceBytes = 0 },
		"HTTP timeout":        func(l *Limits) { l.HTTP.Timeout = 0 },
		"HTTP requests":       func(l *Limits) { l.HTTP.MaxRequests = 0 },
		"HTTP responses":      func(l *Limits) { l.HTTP.MaxResponseBytes = 0 },
		"HTTP stream":         func(l *Limits) { l.HTTP.MaxStreamBytes = 0 },
		"HTTP headers":        func(l *Limits) { l.HTTP.MaxHeaderBytes = 0 },
		"request bytes":       func(l *Limits) { l.MaxRequestBytes = 0 },
		"pending workers":     func(l *Limits) { l.MaxPendingOperations = 0 },
		"result envelope":     func(l *Limits) { l.MaxEncodedResultBytes = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			policy := policyLimits()
			change(&policy)
			definition := compiledDefinition("async function execute() { return true; }")
			definition.Limits = &toolschema.ExecutionLimits{
				TimeoutMillis: util.ToPtr(int64(100)), MaxRequests: util.ToPtr(1),
				MaxInputBytes: util.ToPtr(int64(100)), MaxOutputBytes: util.ToPtr(int64(100)),
				MaxResponseBytes: util.ToPtr(int64(100)), MaxEncodedResultBytes: util.ToPtr(int64(100)),
			}
			// Positive authored caps still leave invalid operator policy invalid.
			program, err := Compile(definition, nil, policy)
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// TestCompileAndValidateHaveSeparatePhases proves that compilation runs neither
// library nor Tool initialization, and validation never invokes execute.
func TestCompileAndValidateHaveSeparatePhases(t *testing.T) {
	for name, source := range map[string]struct{ script, library string }{
		"script throw":    {`throw "private script"; async function execute() {}`, ""},
		"library throw":   {`async function execute() {}`, `throw "private library";`},
		"missing context": {`context.connections[0].fetch("https://example.invalid"); async function execute() {}`, ""},
		"global fetch":    {`fetch("https://example.invalid"); async function execute() {}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			program, err := Compile(compiledDefinition(source.script), compiledLibrary(t, source.library), policyLimits())
			require.NoError(t, err)
			// Initialization errors are classified without exposing source or values.
			err = program.Validate(context.Background())
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private")
		})
	}

	// Only pure helpers are available, and an execute body that throws is not run.
	program, err := Compile(compiledDefinition(`
		if (typeof params !== "undefined" || typeof cfg !== "undefined" ||
		    typeof context !== "undefined" || typeof fetch !== "undefined" ||
		    typeof console !== "undefined") throw "unexpected capability";
		const ready = toolResult({structuredContent: btoa("ready")});
		const failure = new ToolError("FAILED", "Failed");
		async function execute() { throw "execute must not run"; }
	`), nil, policyLimits())
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
}

// TestCompileChecksSelectedLibrary bounds the combined source and applies Tool
// host restrictions to the selected connector helpers before initialization.
func TestCompileChecksSelectedLibrary(t *testing.T) {
	definition := compiledDefinition("async function execute() { return selected(); }")
	library := compiledLibrary(t, `function selected() { return "chosen"; }`)
	limits := policyLimits()
	limits.Runtime.MaxSourceBytes = len(*definition.Javascript) + len(library.Source())

	// Exactly the combined allowance succeeds; neither source gets its own cap.
	program, err := Compile(definition, library, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	limits.Runtime.MaxSourceBytes--
	program, err = Compile(definition, library, limits)
	require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
	require.Nil(t, program)

	for _, source := range []string{`var fetch;`, `function execute() {}`} {
		// Generic library parsing is permissive; Tool compilation adds its own policy.
		program, err = Compile(definition, compiledLibrary(t, source), policyLimits())
		require.ErrorIs(t, err, apjsruntime.ErrInvalidSource)
		require.Nil(t, program)
	}
}

// TestCompileCapturesInputs keeps schemas, selected code, and effective policy
// stable after the caller edits definition buffers, pointers, and library storage.
func TestCompileCapturesInputs(t *testing.T) {
	definition := compiledDefinition(`if (selected !== "original") throw "changed library";
		async function execute() { return {saved: true}; }`)
	definition.Limits = &toolschema.ExecutionLimits{MaxRequests: util.ToPtr(2)}
	library := compiledLibrary(t, `const selected = "original";`)
	limits := policyLimits()
	program, err := Compile(definition, library, limits)
	require.NoError(t, err)

	// Caller mutation cannot change an already captured revision or its code.
	*definition.Javascript = `throw "changed script"; async function execute() {}`
	for i := range definition.InputSchema {
		definition.InputSchema[i] = 'x'
	}
	for i := range definition.OutputSchema {
		definition.OutputSchema[i] = 'x'
	}
	*definition.Limits.MaxRequests = 100
	limits.Runtime.MaxWorkUnits = 1
	*library = *compiledLibrary(t, `const selected = "replacement";`)
	require.NoError(t, program.Validate(context.Background()))
	require.Equal(t, 2, program.limits.HTTP.MaxRequests)
	require.Equal(t, policyLimits().Runtime.MaxWorkUnits, program.limits.Runtime.MaxWorkUnits)

	// Both captured schemas continue to enforce their original contracts.
	_, err = program.schemas.ValidateInput(context.Background(), common.RawJSON(`{"id":"customer-7"}`))
	require.NoError(t, err)
	_, err = program.schemas.ValidateInput(context.Background(), common.RawJSON(`{"id":7}`))
	require.ErrorIs(t, err, validation.ErrInvalidInput)
	require.NoError(t, program.schemas.ValidateOutput(context.Background(), common.RawJSON(`{"saved":true}`)))
	require.ErrorIs(t, program.schemas.ValidateOutput(context.Background(), common.RawJSON(`{"saved":"yes"}`)), validation.ErrInvalidOutput)
}

// TestValidateUsesEffectiveRuntimePolicy distinguishes policy exhaustion from
// caller cancellation while retaining no initialized VM between checks.
func TestValidateUsesEffectiveRuntimePolicy(t *testing.T) {
	limits := policyLimits()
	definition := compiledDefinition(`while (true) {} async function execute() {}`)
	definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(20))}
	program, err := Compile(definition, nil, limits)
	require.NoError(t, err)
	// Authored timeout lowers the runtime policy and stops busy initialization.
	require.ErrorIs(t, program.Validate(context.Background()), apjsruntime.ErrLimitExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, program.Validate(ctx), context.Canceled)
	ctx, cancel = context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	require.ErrorIs(t, program.Validate(ctx), context.DeadlineExceeded)

	for _, empty := range []*Program{nil, {}} {
		// Missing internal programs and contexts never bypass initialization policy.
		require.ErrorIs(t, empty.Validate(context.Background()), apjsruntime.ErrInvalidInput)
	}
	require.ErrorIs(t, program.Validate(nil), apjsruntime.ErrInvalidInput)
}

// TestValidateConcurrentRuntimes ensures repeated publication checks do not
// share authored state, including state introduced by a connector library.
func TestValidateConcurrentRuntimes(t *testing.T) {
	library := compiledLibrary(t, `let calls = 0; function initialize() { if (++calls !== 1) throw "reused"; }`)
	program, err := Compile(compiledDefinition(`initialize(); async function execute() { return true; }`), library, policyLimits())
	require.NoError(t, err)
	var workers sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() { defer workers.Done(); errors <- program.Validate(context.Background()) }()
	}
	workers.Wait()
	close(errors)
	// Every check gets a fresh library/Tool scope under the same copied policy.
	for err := range errors {
		require.NoError(t, err)
	}
	require.NoError(t, program.Validate(context.Background()))
}

// TestCompileAllowsAbsentOrBooleanOutputSchema preserves schema omission and
// boolean contracts rather than conflating a rejecting schema with no check.
func TestCompileAllowsAbsentOrBooleanOutputSchema(t *testing.T) {
	for _, schema := range []string{"", "true", "false"} {
		definition := compiledDefinition("async function execute() { return 1; }")
		definition.OutputSchema = common.RawJSON(schema)
		program, err := Compile(definition, nil, policyLimits())
		require.NoError(t, err)
		// Initialization validates the program, not the result of an uncalled execute.
		require.NoError(t, program.Validate(context.Background()))
		err = program.schemas.ValidateOutput(context.Background(), common.RawJSON(`1`))
		if schema == "false" {
			require.ErrorIs(t, err, validation.ErrInvalidOutput)
		} else {
			require.NoError(t, err)
		}
	}
}
