package javascript_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// evaluateTool checks that a successful invocation produces detached JSON.
func evaluateTool(t *testing.T, program *javascript.ToolProgram, input javascript.Input) common.RawJSON {
	t.Helper()
	result, err := program.Evaluate(context.Background(), input)
	require.NoError(t, err)
	require.True(t, json.Valid(result))
	return result
}

// TestToolEvaluateCapturedInput exercises published helpers, entry-point arguments, and copied aliases together.
func TestToolEvaluateCapturedInput(t *testing.T) {
	generation, catalog := uint64(7), uint64(11)
	input := javascript.Input{
		Context: expressionContext(t, render.Input{
			Params: map[string]any{
				"name":  "Ada",
				"count": 3,
			},
			Cfg: map[string]any{
				"tenant": "acme",
				"nested": map[string]any{
					"region": "us",
				},
			},
			Labels:              map[string]string{"team": "blue"},
			Annotations:         map[string]string{"note": "captured"},
			ConnectionID:        "cxn_example",
			ConnectionNamespace: "root.product.owner",
			ToolID:              "tol_example",
			Revision:            9,
			ToolSetID:           "tls_example",
			ToolSetGeneration:   &generation,
			CatalogRevision:     &catalog,
		}),
		Response: common.RawJSON(`{"status":200,"body":{"id":"contact-1"}}`),
	}

	// Publication initializes helpers without invocation data, so cfg is read only when called.
	library, err := apjs.CompileAndValidateLibrary(`
		let calls = 0;
		function greet(name) { calls++; return name + "@" + cfg.tenant; }
	`)
	require.NoError(t, err)

	program := compileTool(t, `
		async function execute(args, snapshot) {
			const result = {
				greeting: greet(args.name), 
                count: args.count, 
                calls: calls,
				connection: snapshot.connection, 
                tool: snapshot.tool,
				region: cfg.nested.region, 
                label: labels.team, 
                annotation: annotations.note,
				responseID: response.body.id,
				aliases: args === params && snapshot === context && cfg === context.cfg &&
					labels === context.labels && annotations === context.annotations,
				paramsSeparate: !("params" in snapshot),
				nativeObjects: Object.getPrototypeOf(args) === Object.prototype &&
					Object.getPrototypeOf(cfg) === Object.prototype
			};

			args.name = "changed"; 
            args.count = 99;
			cfg.nested.region = "changed"; 
            labels.team = "changed";
			annotations.note = "changed"; 
            response.body.id = "changed";

			return result;
		}
	`, library)

	// The complete Tool must also validate before any connection snapshot is supplied.
	require.NoError(t, program.Validate(context.Background()))

	// Every invocation starts from the captured data and a fresh helper-library state.
	for range 2 {
		result := evaluateTool(t, program, input)
		require.JSONEq(t, `{
			"greeting": "Ada@acme",
            "count": 3,
            "calls": 1,
			"connection": {
              "id": "cxn_example",
              "namespace": "root.product.owner"
             },
			"tool": {
               "id": "tol_example",
               "revision": 9,
               "toolSetId": "tls_example",
               "toolSetGeneration": 7,
               "catalogRevision": 11
            },
			"region": "us",
            "label": "blue",
            "annotation": "captured",
            "responseID": "contact-1",
			"aliases": true,
            "paramsSeparate": true,
            "nativeObjects": true
		}`, string(result))

		// Mutating returned bytes also cannot affect a later invocation.
		result[0] = 'x'
	}

	require.JSONEq(t, `{"status":200,"body":{"id":"contact-1"}}`, string(input.Response))
}

// TestToolEvaluateConcurrentRuntimes keeps helper and input mutations private to each invocation.
func TestToolEvaluateConcurrentRuntimes(t *testing.T) {
	library := toolLibrary(t, `let calls = 0; function next() { return ++calls; }`)
	program := compileTool(t, `async function execute(args) {
		args.count++;
		return {count: args.count, calls: await Promise.resolve(next())};
	}`, library)
	input := javascript.Input{Context: expressionContext(t, render.Input{Params: map[string]any{"count": 3}})}

	for i := range 8 {
		t.Run(fmt.Sprintf("invocation-%d", i), func(t *testing.T) {
			t.Parallel()
			// Sharing the compiled program and immutable snapshot must not share VM state.
			require.JSONEq(t, `{"count":4,"calls":1}`, string(evaluateTool(t, program, input)))
		})
	}
}

// TestToolEvaluateNoHostIO keeps ordinary Tool execution separate from the HTTP adapter.
func TestToolEvaluateNoHostIO(t *testing.T) {
	program := compileTool(t, `async function execute() { return {fetch: typeof fetch, connections: context.connections}; }`, nil)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}

	// Loading Tool input alone must not install the connection's outbound capability.
	require.JSONEq(t, `{"fetch":"undefined","connections":[]}`, string(evaluateTool(t, program, input)))
}

// TestToolEvaluateInvalidInputs rejects incomplete invocation state before running authored code.
func TestToolEvaluateInvalidInputs(t *testing.T) {
	program := compileTool(t, `async function execute() { throw "must not run"; }`, nil)
	input := javascript.Input{Context: expressionContext(t, render.Input{})}
	for name, test := range map[string]struct {
		program *javascript.ToolProgram
		ctx     context.Context
		input   javascript.Input
	}{
		"nil program":        {nil, context.Background(), input},
		"empty program":      {&javascript.ToolProgram{}, context.Background(), input},
		"nil context":        {program, nil, input},
		"missing snapshot":   {program, context.Background(), javascript.Input{}},
		"malformed response": {program, context.Background(), javascript.Input{Context: input.Context, Response: common.RawJSON(`{"private":`)}},
	} {
		t.Run(name, func(t *testing.T) {
			// Invalid Go inputs must produce a safe error rather than a panic or script exception.
			result, err := test.program.Evaluate(test.ctx, test.input)
			require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private")
		})
	}
}
