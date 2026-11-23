package jsexec

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestExecuteRejectsInvalidInvocation prevents incomplete internal admission
// inputs from creating a VM or reaching the supplied connection client.
func TestExecuteRejectsInvalidInvocation(t *testing.T) {
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return executeJSONResponse(`true`), nil
	})
	for _, program := range []*Program{nil, {}} {
		result, err := program.Execute(context.Background(), client, executeInvocation())
		require.ErrorIs(t, err, ErrInvalidInvocation)
		require.Nil(t, result)
	}

	program := compileExecute(t, `async function execute() { return true; }`, executeLimits())
	result, err := program.Execute(nil, client, executeInvocation())
	require.ErrorIs(t, err, ErrInvalidInvocation)
	require.Nil(t, result)
	result, err = program.Execute(context.Background(), nil, executeInvocation())
	require.ErrorIs(t, err, ErrInvalidInvocation)
	require.Nil(t, result)

	for name, change := range map[string]func(*Invocation){
		"invocation identity": func(i *Invocation) { i.ID = "invalid" },
		"Tool identity":       func(i *Invocation) { i.Snapshot.ToolID = "" },
		"Tool revision":       func(i *Invocation) { i.Snapshot.Revision = 0 },
		"connection identity": func(i *Invocation) { i.Snapshot.ConnectionID = "invalid" },
	} {
		t.Run(name, func(t *testing.T) {
			// Malformed trusted identities remain Go errors, not model-facing failures.
			invocation := executeInvocation()
			change(&invocation)
			result, err := program.Execute(context.Background(), client, invocation)
			require.ErrorIs(t, err, ErrInvalidInvocation)
			require.Nil(t, result)
		})
	}
	require.Zero(t, calls.Load())
}

// TestExecuteExpiredAdmissionDeadline includes time already spent preparing the
// invocation without treating an elapsed policy deadline as caller cancellation.
func TestExecuteExpiredAdmissionDeadline(t *testing.T) {
	program := compileExecute(t, `async function execute() {
		await context.connections[0].fetch("https://example.test/never"); return true;
	}`, executeLimits())
	invocation := executeInvocation()
	invocation.Deadline = time.Unix(1, 0)
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return executeJSONResponse(`true`), nil
	})

	// The original caller is still live, so return a safe failure with no sends.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	requireExecuteFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
	require.Zero(t, calls.Load())
}

// TestExecutePolicyTimeoutUsesEarlierLimit applies the narrower runtime/HTTP
// timeout to both busy JavaScript and a suspended outbound request.
func TestExecutePolicyTimeoutUsesEarlierLimit(t *testing.T) {
	for _, shorter := range []string{"runtime", "HTTP"} {
		for _, work := range []string{"busy JavaScript", "pending fetch"} {
			t.Run(shorter+"/"+work, func(t *testing.T) {
				limits := executeLimits()
				limits.Runtime.Timeout, limits.HTTP.Timeout = 10*time.Second, 10*time.Second
				if shorter == "runtime" {
					limits.Runtime.Timeout = 200 * time.Millisecond
				} else {
					limits.HTTP.Timeout = 200 * time.Millisecond
				}
				source := `async function execute() { while (true) {} }`
				if work == "pending fetch" {
					source = `async function execute() { return await context.connections[0].fetch("https://example.test/wait"); }`
				}
				program := compileExecute(t, source, limits)
				watchdog, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				invocation := executeInvocation()
				// A later admission deadline cannot enlarge either compiled timeout.
				invocation.Deadline, _ = watchdog.Deadline()
				var calls atomic.Int32
				stopped := make(chan struct{})
				client := executeClient(func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					defer close(stopped)
					<-request.Context().Done()
					return nil, request.Context().Err()
				})

				// Policy expiry produces a result while the caller watchdog stays live.
				result, err := program.Execute(watchdog, client, invocation)
				require.NoError(t, err)
				require.NoError(t, watchdog.Err())
				state := api.ToolExecutionStateNotStarted
				if work == "pending fetch" {
					state = api.ToolExecutionStateMayHaveExecuted
					require.EqualValues(t, 1, calls.Load())
					select {
					case <-stopped:
					case <-watchdog.Done():
						t.Fatal("outbound worker did not observe cancellation")
					}
				} else {
					require.Zero(t, calls.Load())
				}
				requireExecuteFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", state)
			})
		}
	}
}

// executeOutcome transfers the detached public result and Go error from an
// execution goroutine so cancellation can be synchronized with client admission.
type executeOutcome struct {
	result *api.ToolResult
	err    error
}

// TestExecuteCallerCancellation interrupts both waiting I/O and busy JavaScript
// after a connection handoff, retaining the caller's Go context classification.
func TestExecuteCallerCancellation(t *testing.T) {
	for _, busy := range []bool{false, true} {
		name := "pending fetch"
		if busy {
			name = "busy JavaScript"
		}
		t.Run(name, func(t *testing.T) {
			limits := executeLimits()
			limits.Runtime.Timeout, limits.HTTP.Timeout = 10*time.Second, 10*time.Second
			source := `async function execute() { await context.connections[0].fetch("https://example.test/wait"); return true; }`
			if busy {
				source = `async function execute() { context.connections[0].fetch("https://example.test/wait"); while (true) {} }`
			}
			program := compileExecute(t, source, limits)
			watchdog, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			ctx, cancel := context.WithCancel(watchdog)
			defer cancel()
			started, stopped := make(chan struct{}), make(chan struct{})
			client := executeClient(func(request *http.Request) (*http.Response, error) {
				close(started)
				defer close(stopped)
				<-request.Context().Done()
				return nil, request.Context().Err()
			})
			finished := make(chan executeOutcome, 1)
			go func() {
				result, err := program.Execute(ctx, client, executeInvocation())
				finished <- executeOutcome{result: result, err: err}
			}()

			// Cancel only after the configured client receives the request.
			select {
			case <-started:
			case <-watchdog.Done():
				t.Fatal("outbound request was not admitted")
			}
			cancel()
			select {
			case outcome := <-finished:
				require.ErrorIs(t, outcome.err, context.Canceled)
				require.Nil(t, outcome.result)
			case <-watchdog.Done():
				t.Fatal("execution did not observe caller cancellation")
			}
			select {
			case <-stopped:
			case <-watchdog.Done():
				t.Fatal("outbound worker did not stop")
			}
		})
	}

	// An already elapsed caller deadline is also a Go error and cannot reach I/O.
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	program := compileExecute(t, `async function execute() { return true; }`, executeLimits())
	result, err := program.Execute(ctx, executeClient(func(*http.Request) (*http.Response, error) {
		return executeJSONResponse(`true`), nil
	}), executeInvocation())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}

// TestExecuteInitializationCannotSend keeps initialization and its microtasks
// outside the connection's I/O phase even during an admitted execution.
func TestExecuteInitializationCannotSend(t *testing.T) {
	for _, source := range []string{
		`context.connections[0].fetch("https://example.test/never"); async function execute() { return true; }`,
		`Promise.resolve().then(() => context.connections[0].fetch("https://example.test/never")); async function execute() { return true; }`,
	} {
		program := compileExecute(t, source, executeLimits())
		var calls atomic.Int32
		client := executeClient(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return executeJSONResponse(`true`), nil
		})

		// The attempted capability use fails opaquely and never dispatches a worker.
		result, err := program.Execute(context.Background(), client, executeInvocation())
		require.NoError(t, err)
		requireExecuteFailure(t, result, "OUTPUT_INVALID", api.ToolExecutionStateNotStarted)
		require.Zero(t, calls.Load())
	}
}

// TestExecuteCatchableHTTPAdmissionCaps keeps rejected requests outside the
// client while allowing authored code to handle each local fetch rejection.
func TestExecuteCatchableHTTPAdmissionCaps(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Limits)
		source string
		calls  int32
	}{
		{"request count", func(l *Limits) { l.HTTP.MaxRequests = 1 }, `async function execute() {
			await context.connections[0].fetch("https://example.test/one");
			try { await context.connections[0].fetch("https://example.test/two"); }
			catch (error) { return true; } return false;
		}`, 1},
		{"request bytes", func(l *Limits) { l.MaxRequestBytes = 128 }, `async function execute() {
			try { await context.connections[0].fetch("https://example.test/too-large", {method:"POST", body:"x".repeat(129)}); }
			catch (error) { return true; } return false;
		}`, 0},
		{"pending workers", func(l *Limits) { l.MaxPendingOperations = 1 }, `async function execute() {
			const first = context.connections[0].fetch("https://example.test/one"); let refused = false;
			try { await context.connections[0].fetch("https://example.test/two"); }
			catch (error) { refused = true; } await first; return refused;
		}`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := executeLimits()
			test.change(&limits)
			program := compileExecute(t, test.source, limits)
			var calls atomic.Int32
			client := executeClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return executeJSONResponse(`true`), nil
			})

			// A handled rejection does not poison a later valid result or leak workers.
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.JSONEq(t, `true`, string(result.StructuredContent))
			require.Equal(t, test.calls, calls.Load())
		})
	}
}

// TestExecuteAggregateResponseLimitIsTerminal cannot recover an exhausted shared
// response allowance by catching a body-read failure in authored JavaScript.
func TestExecuteAggregateResponseLimitIsTerminal(t *testing.T) {
	limits := executeLimits()
	limits.HTTP.MaxResponseBytes = 5
	program := compileExecute(t, `async function execute() {
		await (await context.connections[0].fetch("https://example.test/one")).text();
		try { await (await context.connections[0].fetch("https://example.test/two")).text(); }
		catch (error) { return "caught"; }
	}`, limits)
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return executeJSONResponse(`123`), nil
	})

	// The second read crosses the aggregate cap after both connection handoffs.
	result, err := program.Execute(context.Background(), client, executeInvocation())
	require.NoError(t, err)
	requireExecuteFailure(t, result, "OUTPUT_TOO_LARGE", api.ToolExecutionStateMayHaveExecuted)
	require.EqualValues(t, 2, calls.Load())
}

// TestExecuteNativeLimitsRemainTerminal covers independent native work/storage
// and output limits without exposing partially converted results.
func TestExecuteNativeLimitsRemainTerminal(t *testing.T) {
	for name, change := range map[string]func(*Limits){
		"work":       func(l *Limits) { l.Runtime.MaxWorkUnits = 1 },
		"allocation": func(l *Limits) { l.Runtime.MaxAllocationUnits = 1 },
		"output":     func(l *Limits) { l.Runtime.MaxOutputBytes = 4 },
	} {
		t.Run(name, func(t *testing.T) {
			limits := executeLimits()
			change(&limits)
			program := compileExecute(t, `async function execute() { return "private-output"; }`, limits)
			client := executeClient(func(*http.Request) (*http.Response, error) { return executeJSONResponse(`true`), nil })

			// The finite native allowance fails before any connection call.
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			requireExecuteFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
		})
	}

	// An enclosing execution's remaining ledger cannot be reset by this executor.
	limits := executeLimits()
	outerLimits := limits.Runtime
	outerLimits.MaxWorkUnits = 1
	ctx, err := apjsruntime.WithExecutionBudget(context.Background(), outerLimits)
	require.NoError(t, err)
	require.NoError(t, apjsruntime.ChargeWork(ctx, 1))
	program := compileExecute(t, `async function execute() { return true; }`, limits)
	result, err := program.Execute(ctx, executeClient(func(*http.Request) (*http.Response, error) { return executeJSONResponse(`true`), nil }), executeInvocation())
	require.NoError(t, err)
	requireExecuteFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
}

// TestExecuteValidatesOutputAfterVMShutdown keeps the enclosing work context
// live after EvaluateWithHTTP closes its dedicated HTTP session.
func TestExecuteValidatesOutputAfterVMShutdown(t *testing.T) {
	definition := executeDefinition(`async function execute() {
		return await (await context.connections[0].fetch("https://example.test/customer")).json();
	}`)
	definition.OutputSchema = common.RawJSON(`{"type":"object","required":["id"],"properties":{"id":{"const":"customer-42"}}}`)
	program, err := Compile(definition, nil, executeLimits())
	require.NoError(t, err)
	client := executeClient(func(*http.Request) (*http.Response, error) {
		return executeJSONResponse(`{"id":"customer-42"}`), nil
	})

	// Successful schema validation and envelope assembly must not see ErrClosed.
	result, err := program.Execute(context.Background(), client, executeInvocation())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"id":"customer-42"}`, string(result.StructuredContent))

	// A larger otherwise-valid output fails the schema data allowance after send.
	limits := executeLimits()
	limits.Validation.MaxOutputBytes = 4
	definition.OutputSchema = common.RawJSON(`true`)
	program, err = Compile(definition, nil, limits)
	require.NoError(t, err)
	result, err = program.Execute(context.Background(), client, executeInvocation())
	require.NoError(t, err)
	requireExecuteFailure(t, result, "OUTPUT_TOO_LARGE", api.ToolExecutionStateMayHaveExecuted)
}

// TestExecuteArgumentBytesRemainSeparateFromConfiguration permits a small caller
// payload while retaining the independently bounded admitted connection metadata.
func TestExecuteArgumentBytesRemainSeparateFromConfiguration(t *testing.T) {
	limits := executeLimits()
	limits.Validation.MaxInputBytes = 2
	program := compileExecute(t, `async function execute() { return context.connections[0].cfg.tenant; }`, limits)
	invocation := executeInvocation()
	tenant := strings.Repeat("tenant-", 64)
	invocation.Snapshot.Cfg = map[string]any{"tenant": tenant}
	client := executeClient(func(*http.Request) (*http.Response, error) { return executeJSONResponse(`true`), nil })

	// Exactly two argument bytes are valid even when configuration is much larger.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, tenant, *result.Content[0].Text)

	// Whitespace is still part of the raw argument allowance and fails before I/O.
	invocation.Arguments = common.RawJSON(`{} `)
	result, err = program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	requireExecuteFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
}
