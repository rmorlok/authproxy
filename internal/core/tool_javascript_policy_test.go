package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools"
	"github.com/rmorlok/authproxy/internal/tools/jsexec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// configuredJavaScriptPolicy resolves operator settings without database or
// connection dependencies, as the later invocation dispatcher will do.
func configuredJavaScriptPolicy(t *testing.T, settings *sconfig.ToolInvocationLimits) jsexec.Limits {
	t.Helper()
	s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
	limits, err := s.toolJavaScriptInvocationPolicy()
	require.NoError(t, err)
	return limits
}

// TestToolJavaScriptInvocationPolicyDefaults supplies every executor allowance
// even when an embedded caller omits any level of configuration.
func TestToolJavaScriptInvocationPolicyDefaults(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  config.C
	}{
		{"nil config", nil},
		{"nil root", config.FromRoot(nil)},
		{"absent tools", config.FromRoot(&sconfig.Root{})},
		{"absent invocation", config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{}})},
		{"empty invocation", config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: &sconfig.ToolInvocationLimits{}}})},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Shared settings agree with HTTP execution; JavaScript adds its own finite capabilities.
			s := &service{cfg: test.cfg}
			got, err := s.toolJavaScriptInvocationPolicy()
			require.NoError(t, err)
			httpLimits, err := s.toolHTTPInvocationPolicy()
			require.NoError(t, err)
			require.Equal(t, httpLimits.Render, got.Render)
			require.Equal(t, httpLimits.Validation, got.Validation)
			require.Equal(t, httpLimits.HTTP, got.HTTP)
			require.Equal(t, httpLimits.Response.MaxEncodedResultBytes, got.MaxEncodedResultBytes)
			require.Equal(t, 256<<10, got.Runtime.MaxSourceBytes)
			require.Equal(t, 16<<20, got.Runtime.MaxInputBytes)
			require.Equal(t, 8<<20, got.Runtime.MaxOutputBytes)
			require.Equal(t, 64, got.Runtime.MaxDepth)
			require.Equal(t, 1_000_000, got.Runtime.MaxNodes)
			require.Equal(t, 256, got.Runtime.MaxStackDepth)
			require.Equal(t, time.Minute, got.Runtime.Timeout)
			require.Equal(t, 1<<20, got.MaxRequestBytes)
			require.Equal(t, 8, got.MaxPendingOperations)
			wantUnits := 16*((1<<20)+(16<<20)+(8<<20)+(8<<20)+20*((1<<20)+(64<<10))) + 32*1_000_000
			require.Equal(t, wantUnits, got.Runtime.MaxWorkUnits)
			require.Equal(t, wantUnits, got.Runtime.MaxAllocationUnits)

			// The resolved policy compiles and initializes without hidden caller-supplied limits.
			definition := toolschema.ToolDefinition{InputSchema: common.RawJSON(`{"type":"object"}`),
				Javascript: util.ToPtr(`async function execute() { return true; }`)}
			program, err := jsexec.Compile(definition, nil, got)
			require.NoError(t, err)
			require.NoError(t, program.Validate(context.Background()))
			got.MaxPendingOperations = 1
			again, err := s.toolJavaScriptInvocationPolicy()
			require.NoError(t, err)
			require.Equal(t, 8, again.MaxPendingOperations)
		})
	}
}

// TestToolJavaScriptInvocationPolicyConfigured maps shared and JavaScript-only
// overrides independently and retains no mutable configuration pointers.
func TestToolJavaScriptInvocationPolicyConfigured(t *testing.T) {
	settings := &sconfig.ToolInvocationLimits{
		Timeout: &sconfig.HumanDuration{Duration: 3 * time.Second}, MaxRequests: util.ToPtr(4),
		MaxInputBytes: testToolPolicyBytes(t, "2kib"), MaxResponseBytes: testToolPolicyBytes(t, "3kib"),
		MaxOutputBytes: testToolPolicyBytes(t, "4kib"), MaxEncodedResultBytes: testToolPolicyBytes(t, "5kib"),
		MaxStackDepth: util.ToPtr(48), MaxRenderBytes: testToolPolicyBytes(t, "6kib"),
		MaxJavascriptSourceBytes: testToolPolicyBytes(t, "7kib"), MaxJavascriptInputBytes: testToolPolicyBytes(t, "8kib"),
		MaxRequestBytes: testToolPolicyBytes(t, "9kib"), MaxPendingOperations: util.ToPtr(2),
		MaxResponseStreamBytes: testToolPolicyBytes(t, "10kib"), MaxResponseHeaderBytes: testToolPolicyBytes(t, "11kib"),
		MaxDepth: util.ToPtr(12), MaxValues: util.ToPtr(3000), MaxRenderSteps: util.ToPtr(4000), MaxNumberDigits: util.ToPtr(80),
		MaxTransformInputBytes: testToolPolicyBytes(t, "13kib"), MaxResponseSourceBytes: testToolPolicyBytes(t, "14kib"),
	}
	got := configuredJavaScriptPolicy(t, settings)
	s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
	httpLimits, err := s.toolHTTPInvocationPolicy()
	require.NoError(t, err)

	// Shared policy agrees with the existing executor without borrowing transform-specific caps.
	require.Equal(t, httpLimits.Render, got.Render)
	require.Equal(t, httpLimits.Validation, got.Validation)
	require.Equal(t, httpLimits.HTTP, got.HTTP)
	require.Equal(t, httpLimits.Response.MaxEncodedResultBytes, got.MaxEncodedResultBytes)
	require.Equal(t, 7<<10, got.Runtime.MaxSourceBytes)
	require.Equal(t, 8<<10, got.Runtime.MaxInputBytes)
	require.Equal(t, 4<<10, got.Runtime.MaxOutputBytes)
	require.Equal(t, 12, got.Runtime.MaxDepth)
	require.Equal(t, 3000, got.Runtime.MaxNodes)
	require.Equal(t, 48, got.Runtime.MaxStackDepth)
	require.Equal(t, 3*time.Second, got.Runtime.Timeout)
	require.Equal(t, 9<<10, got.MaxRequestBytes)
	require.Equal(t, 2, got.MaxPendingOperations)
	wantUnits := 16*((2+8+4+3+4*(9+11))<<10) + 32*3000
	require.Equal(t, wantUnits, got.Runtime.MaxWorkUnits)
	require.Equal(t, wantUnits, got.Runtime.MaxAllocationUnits)

	// Captured policies remain stable when an operator replaces pointer-backed settings.
	original := got
	settings.Timeout.Duration = time.Hour
	*settings.MaxPendingOperations = 99
	require.NoError(t, json.Unmarshal([]byte(`"99mib"`), settings.MaxJavascriptInputBytes))
	require.Equal(t, original, got)
}

// TestToolJavaScriptInvocationPolicyValidation rejects invalid or overflowing
// JavaScript policy without coupling it to independent HTTP transform allowances.
func TestToolJavaScriptInvocationPolicyValidation(t *testing.T) {
	large := fmt.Sprintf("%db", uint64(1)<<(strconv.IntSize-3))
	for _, test := range []struct {
		name     string
		settings *sconfig.ToolInvocationLimits
	}{
		{"disabled request bytes", &sconfig.ToolInvocationLimits{MaxRequestBytes: testToolPolicyBytes(t, "0b")}},
		{"disabled workers", &sconfig.ToolInvocationLimits{MaxPendingOperations: util.ToPtr(0)}},
		{"disabled source", &sconfig.ToolInvocationLimits{MaxJavascriptSourceBytes: testToolPolicyBytes(t, "0b")}},
		{"argument budget overflow", &sconfig.ToolInvocationLimits{MaxInputBytes: testToolPolicyBytes(t, large)}},
		{"VM input budget overflow", &sconfig.ToolInvocationLimits{MaxJavascriptInputBytes: testToolPolicyBytes(t, large)}},
		{"response budget overflow", &sconfig.ToolInvocationLimits{MaxResponseBytes: testToolPolicyBytes(t, large)}},
		{"request budget overflow", &sconfig.ToolInvocationLimits{MaxRequestBytes: testToolPolicyBytes(t, large)}},
		{"request count overflow", &sconfig.ToolInvocationLimits{MaxRequests: util.ToPtr(math.MaxInt)}},
		{"node budget overflow", &sconfig.ToolInvocationLimits{MaxValues: util.ToPtr(math.MaxInt)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Policy errors surface even when an embedding skipped root configuration validation.
			s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: test.settings}})}
			_, err := s.toolJavaScriptInvocationPolicy()
			require.Error(t, err)
		})
	}

	// Transform-only dimensions do not participate in JavaScript Tool policy derivation.
	settings := &sconfig.ToolInvocationLimits{MaxTransformInputBytes: testToolPolicyBytes(t, large),
		MaxResponseSourceBytes: testToolPolicyBytes(t, large)}
	require.NoError(t, settings.Validate(nil))
	s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
	_, err := s.toolHTTPInvocationPolicy()
	require.Error(t, err)
	got, err := s.toolJavaScriptInvocationPolicy()
	require.NoError(t, err)
	require.Equal(t, configuredJavaScriptPolicy(t, nil), got)
}

// TestToolJavaScriptBudgetSumBoundary checks the remaining allowance before
// addition, even when each individual scaled term fits the runtime integer.
func TestToolJavaScriptBudgetSumBoundary(t *testing.T) {
	// Minimal positive dimensions consume six byte terms and one node term.
	limits := jsexec.Limits{MaxRequestBytes: 1}
	limits.Validation.MaxInputBytes = 1
	limits.Runtime.MaxInputBytes, limits.Runtime.MaxOutputBytes, limits.Runtime.MaxNodes = 1, 1, 1
	limits.HTTP.MaxRequests, limits.HTTP.MaxHeaderBytes, limits.HTTP.MaxResponseBytes = 1, 1, 1
	units, err := toolJavaScriptBudget(limits)
	require.NoError(t, err)
	require.Equal(t, 128, units)

	// A representable argument term leaves exactly fifteen units unused.
	limits.Validation.MaxInputBytes = math.MaxInt/16 - 7
	units, err = toolJavaScriptBudget(limits)
	require.NoError(t, err)
	require.Equal(t, math.MaxInt-15, units)

	// One more header byte costs sixteen units and must fail instead of wrapping.
	limits.HTTP.MaxHeaderBytes++
	_, err = toolJavaScriptBudget(limits)
	require.Error(t, err)
}

// TestToolJavaScriptBudgetScalesWithTraffic verifies cumulative managed work
// grows with each independent traffic/data dimension, without integer wrapping.
func TestToolJavaScriptBudgetScalesWithTraffic(t *testing.T) {
	base := configuredJavaScriptPolicy(t, nil)
	for _, test := range []struct {
		name   string
		change func(*jsexec.Limits)
	}{
		{"arguments", func(l *jsexec.Limits) { l.Validation.MaxInputBytes++ }},
		{"VM input", func(l *jsexec.Limits) { l.Runtime.MaxInputBytes++ }},
		{"output", func(l *jsexec.Limits) { l.Runtime.MaxOutputBytes++ }},
		{"responses", func(l *jsexec.Limits) { l.HTTP.MaxResponseBytes++ }},
		{"request bytes", func(l *jsexec.Limits) { l.MaxRequestBytes++ }},
		{"request count", func(l *jsexec.Limits) { l.HTTP.MaxRequests++ }},
		{"headers", func(l *jsexec.Limits) { l.HTTP.MaxHeaderBytes++ }},
		{"values", func(l *jsexec.Limits) { l.Runtime.MaxNodes++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Raising a supported input or traffic ceiling cannot leave a fixed hidden budget behind.
			changed := base
			test.change(&changed)
			units, err := toolJavaScriptBudget(changed)
			require.NoError(t, err)
			require.Greater(t, units, base.Runtime.MaxWorkUnits)
		})
	}
}

// javascriptPolicyClient supplies the configured connection capability without
// network I/O; each test controls its request observations and streaming response.
type javascriptPolicyClient func(*http.Request) (*http.Response, error)

// Do transfers request ownership to the test's deterministic dispatcher.
func (c javascriptPolicyClient) Do(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		defer request.Body.Close()
	}
	return c(request)
}

// DoWithLabels preserves the configured-client shape and rejects untested label overrides.
func (c javascriptPolicyClient) DoWithLabels(request *http.Request, labels map[string]string) (*http.Response, error) {
	if len(labels) != 0 {
		return nil, errors.New("unexpected request labels")
	}
	return c.Do(request)
}

// javascriptPolicyInvocation is a captured connection with configuration much
// larger than its two-byte argument, exercising separate preparation allowances.
func javascriptPolicyInvocation() tools.Invocation {
	return tools.Invocation{ID: "tiv_01example0000001", Arguments: common.RawJSON(`{}`),
		Snapshot: render.Input{ToolID: "tol_01example0000001", Revision: 7,
			ConnectionID: "cxn_01example0000001", ConnectionNamespace: "root.product.owner",
			Cfg: map[string]any{"tenant": strings.Repeat("x", 2048)}}}
}

// TestToolJavaScriptConfiguredExecution exercises operator/authored policy in
// real VM and HTTP conversion work, with no core-dispatch or public-route shortcut.
func TestToolJavaScriptConfiguredExecution(t *testing.T) {
	settings := &sconfig.ToolInvocationLimits{MaxInputBytes: testToolPolicyBytes(t, "2b"),
		MaxJavascriptInputBytes: testToolPolicyBytes(t, "8kib"), MaxOutputBytes: testToolPolicyBytes(t, "1kib"),
		MaxRequestBytes: testToolPolicyBytes(t, "256b"), MaxRequests: util.ToPtr(2), MaxPendingOperations: util.ToPtr(1)}
	policy := configuredJavaScriptPolicy(t, settings)
	for _, authoredRequests := range []int{1, 100} {
		t.Run(fmt.Sprintf("authored requests %d", authoredRequests), func(t *testing.T) {
			definition := toolschema.ToolDefinition{InputSchema: common.RawJSON(`{"type":"object"}`),
				Javascript: util.ToPtr(`async function execute(params, context) {
					const [connection] = context.connections;
					const first = await (await connection.fetch("https://example.test/one")).json();
					const second = await (await connection.fetch("https://example.test/two")).json();
					let refused = false;
					try { await connection.fetch("https://example.test/three"); } catch (_) { refused = true; }
					return {total:first.value + second.value, connections:context.connections.length,
						direct:typeof fetch, tenantLength:connection.cfg.tenant.length, refused};
				}`), Limits: &toolschema.ExecutionLimits{MaxRequests: util.ToPtr(authoredRequests)}}
			program, err := jsexec.Compile(definition, nil, policy)
			require.NoError(t, err)
			var calls atomic.Int32
			client := javascriptPolicyClient(func(*http.Request) (*http.Response, error) {
				value := calls.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"value":%d}`, value)))}, nil
			})

			// The smaller authored count wins; large authored caps cannot raise the server ceiling.
			result, err := program.Execute(context.Background(), client, javascriptPolicyInvocation())
			require.NoError(t, err)
			if authoredRequests == 1 {
				require.True(t, result.IsError)
				require.Equal(t, "EXECUTION_LIMIT_EXCEEDED", result.Error.Code)
				require.Equal(t, api.ToolExecutionStateMayHaveExecuted, result.Error.ExecutionState)
				require.EqualValues(t, 1, calls.Load())
			} else {
				require.False(t, result.IsError)
				require.EqualValues(t, 2, calls.Load())
				require.JSONEq(t, `{"total":3,"connections":1,"direct":"undefined","tenantLength":2048,"refused":true}`, string(result.StructuredContent))
			}
		})
	}

	// Authored caps lower selected values without changing independent source/worker/native budgets.
	lowered, err := jsexec.ResolveLimits(policy, &toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(1)),
		MaxOutputBytes: util.ToPtr(int64(64)), TimeoutMillis: util.ToPtr(int64(100)), MaxStackDepth: util.ToPtr(32)})
	require.NoError(t, err)
	require.Equal(t, 1, lowered.Validation.MaxInputBytes)
	require.Equal(t, 64, lowered.Runtime.MaxOutputBytes)
	require.Equal(t, 100*time.Millisecond, lowered.Runtime.Timeout)
	require.Equal(t, 32, lowered.Runtime.MaxStackDepth)
	require.Equal(t, policy.Runtime.MaxInputBytes, lowered.Runtime.MaxInputBytes)
	require.Equal(t, policy.MaxPendingOperations, lowered.MaxPendingOperations)
	require.Equal(t, policy.Runtime.MaxWorkUnits, lowered.Runtime.MaxWorkUnits)
}

// TestToolJavaScriptConfiguredAdmissionCaps checks request-size and worker policy
// before dispatch, including completion slots not yet consumed by the owner VM.
func TestToolJavaScriptConfiguredAdmissionCaps(t *testing.T) {
	for _, test := range []struct {
		name, source string
		calls        int32
	}{
		{"request bytes", `async function execute(params, context) {
			try { await context.connections[0].fetch("https://example.test/large", {method:"POST", body:"x".repeat(257)}); }
			catch (_) { return true; } return false;
		}`, 0},
		{"pending worker", `async function execute(params, context) {
			const first = context.connections[0].fetch("https://example.test/one"); let refused = false;
			try { await context.connections[0].fetch("https://example.test/two"); }
			catch (_) { refused = true; } await first; return refused;
		}`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := configuredJavaScriptPolicy(t, &sconfig.ToolInvocationLimits{
				MaxRequestBytes: testToolPolicyBytes(t, "256b"), MaxPendingOperations: util.ToPtr(1)})
			program, err := jsexec.Compile(toolschema.ToolDefinition{InputSchema: common.RawJSON(`{"type":"object"}`),
				Javascript: &test.source}, nil, policy)
			require.NoError(t, err)
			var calls atomic.Int32
			client := javascriptPolicyClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
			})

			// Catchable admission rejection must not reach the configured connection client.
			result, err := program.Execute(context.Background(), client, javascriptPolicyInvocation())
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.JSONEq(t, `true`, string(result.StructuredContent))
			require.Equal(t, test.calls, calls.Load())
		})
	}
}

// TestToolJavaScriptConfiguredSourceBoundary applies the dedicated combined
// Tool/library allowance, independently of response-rule source configuration.
func TestToolJavaScriptConfiguredSourceBoundary(t *testing.T) {
	const source = `async function execute() { return customer(); }`
	const helper = `function customer() { return "customer-42"; }`
	for _, extra := range []int{0, -1} {
		t.Run(fmt.Sprintf("source allowance offset %d", extra), func(t *testing.T) {
			// Compile each part below the cap, then check their combined source at the Tool boundary.
			policy := configuredJavaScriptPolicy(t, &sconfig.ToolInvocationLimits{
				MaxJavascriptSourceBytes: testToolPolicyBytes(t, fmt.Sprintf("%db", len(source)+len(helper)+extra)),
				MaxResponseSourceBytes:   testToolPolicyBytes(t, "1b")})
			library, err := apjs.CompileAndValidateLibraryWithContext(context.Background(), helper, policy.Runtime)
			require.NoError(t, err)
			program, err := jsexec.Compile(toolschema.ToolDefinition{InputSchema: common.RawJSON(`{"type":"object"}`),
				Javascript: util.ToPtr(source)}, library, policy)
			if extra < 0 {
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
				require.Nil(t, program)
			} else {
				require.NoError(t, err)
				require.NoError(t, program.Validate(context.Background()))
			}
		})
	}
}
