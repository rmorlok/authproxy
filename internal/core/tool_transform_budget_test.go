package core

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// configuredTransformPolicy resolves test sizes through the real operator
// configuration path without constructing database or HTTP dependencies.
func configuredTransformPolicy(t *testing.T, inputBytes, outputBytes, nodes int) httpexec.Limits {
	t.Helper()
	settings := &sconfig.ToolInvocationLimits{
		MaxTransformInputBytes: testToolPolicyBytes(t, fmt.Sprintf("%db", inputBytes)),
		MaxOutputBytes:         testToolPolicyBytes(t, fmt.Sprintf("%db", outputBytes)),
		MaxValues:              util.ToPtr(nodes),
	}
	s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
	policy, err := s.toolHTTPInvocationPolicy()
	require.NoError(t, err)
	return policy
}

// TestToolTransformBudgetScalesWithConfiguredLimits keeps independently raised
// byte and node settings from encountering a fixed hidden cumulative ceiling.
func TestToolTransformBudgetScalesWithConfiguredLimits(t *testing.T) {
	baseline := configuredTransformPolicy(t, 16<<10, 8<<10, 256).Response.Transform
	for _, test := range []struct {
		name                 string
		input, output, nodes int
	}{
		{"input", 32 << 10, 8 << 10, 256},
		{"output", 16 << 10, 16 << 10, 256},
		{"nodes", 16 << 10, 8 << 10, 512},
		{"large operator limits", 64 << 20, 32 << 20, 1_000_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Resolve actual settings, rather than constructing a runtime policy directly.
			got := configuredTransformPolicy(t, test.input, test.output, test.nodes).Response.Transform
			require.Greater(t, got.MaxWorkUnits, baseline.MaxWorkUnits)
			require.Greater(t, got.MaxAllocationUnits, baseline.MaxAllocationUnits)
			want, err := toolTransformBudget(got)
			require.NoError(t, err)
			require.Equal(t, want, got.MaxWorkUnits)
			require.Equal(t, want, got.MaxAllocationUnits)
			if test.name == "large operator limits" {
				// The review's 64/32 MiB settings must exceed the former fixed 256 Mi cap.
				require.Greater(t, got.MaxWorkUnits, 256<<20)
				require.Greater(t, got.MaxAllocationUnits, 256<<20)
			}
		})
	}

	// Publication derives its compile-only profile from the same conversion policy.
	publication := toolPublishResponseLimits.Transform
	want, err := toolTransformBudget(publication)
	require.NoError(t, err)
	require.Equal(t, want, publication.MaxWorkUnits)
	require.Equal(t, want, publication.MaxAllocationUnits)
}

// TestToolTransformBudgetRejectsOverflow prevents large, individually valid
// operator limits from wrapping the derived allowance or silently reducing it.
func TestToolTransformBudgetRejectsOverflow(t *testing.T) {
	for _, test := range []struct {
		name                 string
		input, output, nodes int
	}{
		{"input multiplication", math.MaxInt/2 + 1, 1, 1},
		{"output multiplication", 1, math.MaxInt/2 + 1, 1},
		{"node multiplication", 1, 1, math.MaxInt},
		{"byte addition", math.MaxInt/32 + 1, math.MaxInt/32 + 1, 1},
		{"combined terms", math.MaxInt/32 + 1, 1, math.MaxInt/64 + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Each field fits its configuration representation; only derivation overflows.
			settings := &sconfig.ToolInvocationLimits{
				MaxTransformInputBytes: testToolPolicyBytes(t, fmt.Sprintf("%db", test.input)),
				MaxOutputBytes:         testToolPolicyBytes(t, fmt.Sprintf("%db", test.output)),
				MaxValues:              util.ToPtr(test.nodes),
			}
			require.Equal(t, uint64(test.input), settings.MaxTransformInputBytes.Value())
			require.Equal(t, uint64(test.output), settings.MaxOutputBytes.Value())
			require.NoError(t, settings.Validate(nil))
			s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
			_, err := s.toolHTTPInvocationPolicy()
			require.Error(t, err)
		})
	}
}

// TestToolTransformConfiguredByteBoundaries executes input copying, native JSON
// decoding, and result encoding together at the operator's exact byte ceilings.
func TestToolTransformConfiguredByteBoundaries(t *testing.T) {
	policy := configuredTransformPolicy(t, 16<<10, 8<<10, 256)
	limits := policy.Response.Transform
	snapshot, err := render.NewContext(render.Input{}, policy.Render)
	require.NoError(t, err)
	serialized, err := snapshot.SnapshotJSON(context.Background(), render.Limits{
		MaxBytes: limits.MaxInputBytes, MaxDepth: limits.MaxDepth, MaxSteps: limits.MaxNodes,
	})
	require.NoError(t, err)

	// An ASCII field fills the remaining combined snapshot/response allowance.
	prefix, suffix := `{"value":"`, `"}`
	valueBytes := limits.MaxInputBytes - len(serialized) - len(prefix) - len(suffix)
	for _, test := range []struct {
		name                    string
		extraInput, extraOutput int
	}{
		{"exact input and output", 0, 0},
		{"one input byte too many", 1, 0},
		{"one output byte too many", 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := common.RawJSON(prefix + strings.Repeat("x", valueBytes+test.extraInput) + suffix)
			require.Equal(t, limits.MaxInputBytes+test.extraInput, len(serialized)+len(response))
			outputCharacters := limits.MaxOutputBytes - 2 + test.extraOutput
			program, err := javascript.CompileExpression(fmt.Sprintf(`response.value.slice(0, %d)`, outputCharacters), limits)
			require.NoError(t, err)

			// Source, input preparation, and result conversion all use the resolved policy.
			result, err := program.Evaluate(context.Background(), javascript.Input{Context: snapshot, Response: response})
			if test.extraInput != 0 || test.extraOutput != 0 {
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Len(t, result, limits.MaxOutputBytes)
				require.Equal(t, `"`+strings.Repeat("x", outputCharacters)+`"`, string(result))
			}
		})
	}
}

// TestToolTransformConfiguredNodeBoundary covers many small values whose native
// metadata costs must fit the derived allowance independently of their byte size.
func TestToolTransformConfiguredNodeBoundary(t *testing.T) {
	const items = 256
	// The empty snapshot has 13 nodes; the response object/key/array adds three.
	policy := configuredTransformPolicy(t, 4096, 4096, items+16)
	snapshot, err := render.NewContext(render.Input{}, policy.Render)
	require.NoError(t, err)
	serialized, err := snapshot.SnapshotJSON(context.Background(), policy.Render)
	require.NoError(t, err)
	require.JSONEq(t, `{"params":{},"cfg":{},"labels":{},"annotations":{},"connection":{},"tool":{}}`, string(serialized))
	program, err := javascript.CompileExpression(`response.items`, policy.Response.Transform)
	require.NoError(t, err)

	for _, count := range []int{items, items + 1} {
		t.Run(fmt.Sprintf("%d items", count), func(t *testing.T) {
			// Each empty object contributes one input node and exercises output reflection.
			array := `[` + strings.Repeat(`{},`, count-1) + `{}` + `]`
			response := common.RawJSON(`{"items":` + array + `}`)
			require.Less(t, len(serialized)+len(response), policy.Response.Transform.MaxInputBytes)
			result, err := program.Evaluate(context.Background(), javascript.Input{Context: snapshot, Response: response})
			if count == items {
				require.NoError(t, err)
				require.Equal(t, array, string(result))
			} else {
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
				require.Nil(t, result)
			}
		})
	}
}
