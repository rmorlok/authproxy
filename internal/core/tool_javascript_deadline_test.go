package core

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// TestInvokeJavaScriptToolPreparationInterruption keeps decryption inside the
// authored timeout and distinguishes a policy failure from caller cancellation.
func TestInvokeJavaScriptToolPreparationInterruption(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "authored timeout"
		if canceled {
			name = "caller cancellation"
		}
		t.Run(name, func(t *testing.T) {
			// Strict mocks allow the pinned connector load but no configuration or HTTP work.
			f, _ := toolPreparationFixture(t)
			f.definition.ProxyHTTP = nil
			f.definition.Javascript = util.ToPtr(`async function execute() { return true; }`)
			f.definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(250))}
			expectHTTPToolDefinition(t, f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
				DoAndReturn(func(preparation context.Context, _ encfield.EncryptedField) (string, error) {
					if canceled {
						cancel()
					}
					<-preparation.Done()
					return "", preparation.Err()
				})

			// Only a caller cancellation escapes as a Go error; timeout keeps captured identities.
			result, err := f.service.invokeTool(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NoError(t, ctx.Err())
				requireHTTPToolFailure(t, f, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
			}
		})
	}
}

// TestInvokeJavaScriptToolDeadlineAfterSend carries the preparation deadline into
// both pending HTTP and busy JavaScript, retaining effects when policy expires.
func TestInvokeJavaScriptToolDeadlineAfterSend(t *testing.T) {
	for _, name := range []string{"pending HTTP", "busy JavaScript", "caller cancellation"} {
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			f.definition.ProxyHTTP = nil
			source := `async function execute(params, context) {
				await context.connections[0].fetch("https://example.test/update", {method:"POST"}); return true;
			}`
			if name == "busy JavaScript" {
				source = `async function execute(params, context) {
					await context.connections[0].fetch("https://example.test/update", {method:"POST"}); while (true) {}
				}`
			}
			f.definition.Javascript = &source
			f.definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(1000))}
			expectHTTPToolDefinition(t, f)
			var preparationDeadline time.Time
			f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
				DoAndReturn(func(preparation context.Context, _ encfield.EncryptedField) (string, error) {
					var ok bool
					preparationDeadline, ok = preparation.Deadline()
					require.True(t, ok)
					return `{"auth":{"type":"no-auth"}}`, nil
				})
			f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
				Return(`{"tenant":"captured"}`, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observedDeadline := make(chan time.Time, 1)
			transportDone := make(chan struct{})
			var calls atomic.Int32
			expectHTTPToolTransport(t, f, nil, func(r *http.Request) (*http.Response, error) {
				defer close(transportDone)
				calls.Add(1)
				deadline, _ := r.Context().Deadline()
				observedDeadline <- deadline
				if name == "busy JavaScript" {
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`true`))}, nil
				}
				if name == "caller cancellation" {
					cancel()
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})

			// The effect happens before either interruption; no retry or second send is allowed.
			result, err := f.service.invokeTool(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
			if name == "caller cancellation" {
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NoError(t, ctx.Err())
				requireHTTPToolFailure(t, f, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
			}
			// Await cooperative test transport cleanup before inspecting worker observations.
			select {
			case <-transportDone:
			case <-time.After(5 * time.Second):
				t.Fatal("configured transport did not finish")
			}
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, preparationDeadline, <-observedDeadline, "execution must retain the preparation deadline")
		})
	}
}
