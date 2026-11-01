package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/httpf"
	httpmock "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// toolHTTPPolicy supplies explicit operator ceilings for every executor layer.
func toolHTTPPolicy() httpexec.Limits {
	return httpexec.Limits{
		Render: render.Limits{MaxBytes: 32768, MaxDepth: 32, MaxSteps: 10000},
		Validation: validation.Limits{MaxInputBytes: 16384, MaxOutputBytes: 16384,
			MaxDepth: 32, MaxValues: 10000, MaxNumberDigits: 128},
		Response: httpcodec.ResponseLimits{MaxSourceBytes: 4096, MaxResponseBytes: 16384,
			MaxOutputBytes: 16384, MaxEncodedResultBytes: 32768,
			Transform: javascript.Limits{MaxSourceBytes: 4096, MaxInputBytes: 32768,
				MaxOutputBytes: 16384, MaxDepth: 32, MaxNodes: 10000, MaxStackDepth: 128, Timeout: time.Second}},
		HTTP: httphost.Limits{Timeout: 2 * time.Second, MaxRequests: 1,
			MaxResponseBytes: 16384, MaxStreamBytes: 16384, MaxHeaderBytes: 4096},
	}
}

// expectHTTPToolDefinition freezes the fixture's edited definition as captured ciphertext.
func expectHTTPToolDefinition(t *testing.T, f *toolAuthorizationFixture) {
	t.Helper()
	var err error
	f.plaintext, err = json.Marshal(f.definition)
	require.NoError(t, err)
	f.expectCapturedDefinition()
}

// expectHTTPToolPreparation permits the pinned connector and configuration only.
func expectHTTPToolPreparation(f *toolAuthorizationFixture) {
	f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
		Return(`{"auth":{"type":"no-auth"}}`, nil)
	f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
		Return(`{"tenant":"acme/team"}`, nil)
}

// toolHTTPTransport adapts a function to the final network boundary.
type toolHTTPTransport func(*http.Request) (*http.Response, error)

// RoundTrip lets tests observe requests after connection and auth preparation.
func (f toolHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// expectHTTPToolTransport requires one real proxy middleware build, while strict
// mocks reject any database reread, credential lookup, or unplanned second call.
func expectHTTPToolTransport(t *testing.T, f *toolAuthorizationFixture, actorLabels map[string]string, send toolHTTPTransport) {
	t.Helper()
	factory := f.service.httpf.(*httpmock.MockF)
	factory.EXPECT().ForRequestType(httpf.RequestTypeProxy).Return(factory)
	factory.EXPECT().ForConnection(gomock.Any()).DoAndReturn(func(c httpf.Connection) httpf.F {
		require.Equal(t, f.snapshot.Connection.Id, c.GetId())
		require.Equal(t, f.snapshot.Connection.Namespace, c.GetNamespace())
		require.Equal(t, uint64(4), c.GetConnectorGeneration())
		return factory
	})
	factory.EXPECT().ForActor(gomock.Any()).DoAndReturn(func(a httpf.Actor) httpf.F {
		require.Equal(t, actorLabels, a.GetLabels())
		return factory
	})
	factory.EXPECT().ForLabels(gomock.Nil()).Return(factory)
	factory.EXPECT().NewHTTPClient().Return(&http.Client{Transport: send})
}

// requireHTTPToolFailure checks a safe, complete failure carrying the admitted identities.
func requireHTTPToolFailure(t *testing.T, f *toolAuthorizationFixture, result *api.ToolResult, code string, state api.ToolExecutionState) {
	t.Helper()
	require.NotNil(t, result)
	require.NoError(t, result.Validate(nil))
	require.True(t, result.IsError)
	require.Equal(t, code, result.Error.Code)
	require.Equal(t, state, result.Error.ExecutionState)
	require.Equal(t, f.snapshot.Revision.ToolId.String(), result.ToolID)
	require.Equal(t, uint64(7), result.Revision)
	require.Equal(t, f.snapshot.Connection.Id.String(), result.Metadata.ConnectionID)
	require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
	require.Empty(t, result.Content)
	require.Nil(t, result.StructuredContent)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
}

// TestInvokeHTTPToolCapturedExecution passes captured configuration, labels, and
// actor attribution through the actual connection client into rich Tool results.
func TestInvokeHTTPToolCapturedExecution(t *testing.T) {
	var previousID string
	for _, mimeType := range []string{"application/json", "image/png"} {
		t.Run(mimeType, func(t *testing.T) {
			// The original actor is edited during preparation, after authorization copied it.
			f, _ := toolPreparationFixture(t)
			auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
			auth.GetActor().Labels = map[string]string{"team": "captured"}
			f.snapshot.Connection.Labels = database.Labels{"owner": "owner-42"}
			f.definition.ProxyHTTP.URL = "https://example.test/{{path.cfg.tenant}}/{{path.params.id}}"
			f.definition.ProxyHTTP.Headers = map[string]string{"X-Owner": "{{labels.owner}}"}
			expectHTTPToolDefinition(t, f)
			var preparationDeadline time.Time
			f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
				DoAndReturn(func(ctx context.Context, _ encfield.EncryptedField) (string, error) {
					var ok bool
					preparationDeadline, ok = ctx.Deadline()
					require.True(t, ok)
					auth.GetActor().Labels["team"] = "changed after authorization"
					return `{"auth":{"type":"no-auth"}}`, nil
				})
			f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
				Return(`{"tenant":"acme/team"}`, nil)
			calls := 0
			expectHTTPToolTransport(t, f, map[string]string{"team": "captured"}, func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https://example.test/acme%2Fteam/contact%2F42", r.URL.String())
				require.Equal(t, "owner-42", r.Header.Get("X-Owner"))
				deadline, ok := r.Context().Deadline()
				require.True(t, ok)
				require.Equal(t, preparationDeadline, deadline, "preparation must consume the same timeout as execution")
				payload := `{"name":"Ada"}`
				if mimeType == "image/png" {
					payload = string([]byte{0, 255, 128})
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {mimeType}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})

			// A Tool grant without proxy permission is sufficient; the captured revision is returned.
			result, err := f.service.invokeHTTPTool(context.Background(), auth, f.snapshot.Revision.ToolId, util.ToPtr(uint64(7)), common.RawJSON(`{"id":"contact/42"}`), toolHTTPPolicy())
			require.NoError(t, err)
			require.NoError(t, result.Validate(nil))
			require.False(t, result.IsError)
			require.Equal(t, 1, calls)
			require.Equal(t, f.snapshot.Revision.ToolId.String(), result.ToolID)
			require.Equal(t, uint64(7), result.Revision)
			require.Equal(t, f.snapshot.Connection.Id.String(), result.Metadata.ConnectionID)
			require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
			require.NotEqual(t, previousID, result.InvocationID)
			previousID = result.InvocationID
			if mimeType == "application/json" {
				require.JSONEq(t, `{"name":"Ada"}`, string(result.StructuredContent))
			} else {
				require.Nil(t, result.StructuredContent)
				require.Len(t, result.Content, 1)
				require.Equal(t, api.ToolContentTypeImage, result.Content[0].Type)
				require.Equal(t, "image/png", result.Content[0].MIMEType)
				require.Equal(t, "AP+A", *result.Content[0].Data)
			}
		})
	}
}

// TestInvokeHTTPToolPreSendFailures keeps denied and stale lookups out of
// preparation, and classifies admitted failures without building HTTP middleware
// or reaching the upstream transport.
func TestInvokeHTTPToolPreSendFailures(t *testing.T) {
	for _, name := range []string{"denied", "stale revision", "unavailable", "unsupported executor", "unsupported auth", "invalid arguments", "authored input cap", "authored envelope cap", "compile error", "decrypt error"} {
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			verb, input, code := "tool:read", common.RawJSON(`{}`), ""
			var expected *uint64
			var wantErr error
			prepare := true
			switch name {
			case "denied":
				verb, wantErr, prepare = "tool:other", ErrNotFound, false
				expected = util.ToPtr(uint64(99))
			case "stale revision":
				expected, wantErr, prepare = util.ToPtr(uint64(99)), ErrToolRevisionChanged, false
			case "unavailable", "authored envelope cap":
				f.snapshot.Connection.State = database.ConnectionStateDisabled
				code, prepare = "CONNECTION_UNAVAILABLE", false
				if name == "authored envelope cap" {
					f.definition.Limits = &toolschema.ExecutionLimits{MaxEncodedResultBytes: util.ToPtr(int64(8))}
					wantErr = httpcodec.ErrEncodedResultLimit
				}
			case "unsupported executor":
				f.definition.ProxyHTTP = nil
				f.definition.Javascript = util.ToPtr(`async function execute() { return true; }`)
				code, prepare = "UNSUPPORTED_CAPABILITY", false
			case "unsupported auth":
				f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).Return(`{}`, nil)
				code, prepare = "UNSUPPORTED_CAPABILITY", false
			case "invalid arguments":
				input, code = common.RawJSON(`{"private-value":`), "INPUT_INVALID"
			case "authored input cap":
				f.definition.Limits = &toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(2))}
				input, code = common.RawJSON(`{} `), "EXECUTION_LIMIT_EXCEEDED"
			case "compile error":
				f.definition.InputSchema = common.RawJSON(`{"type":"private-invalid"}`)
			case "decrypt error":
				wantErr, prepare = errors.New("private-decryption failure"), false
				f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).Return("", wantErr)
			}
			expectHTTPToolDefinition(t, f)
			if prepare {
				expectHTTPToolPreparation(f)
			}

			// No factory expectation permits accidental sends or eager credential setup.
			result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant(verb), nil), f.snapshot.Revision.ToolId, expected, input, toolHTTPPolicy())
			if wantErr != nil || name == "compile error" {
				require.Error(t, err)
				require.Nil(t, result)
				if wantErr != nil {
					require.ErrorIs(t, err, wantErr)
				}
			} else {
				require.NoError(t, err)
				requireHTTPToolFailure(t, f, result, code, api.ToolExecutionStateNotStarted)
			}
		})
	}
}

// TestInvokeHTTPToolPreparationInterruption separates caller cancellation from
// the authored timeout and never constructs a client after either interruption.
func TestInvokeHTTPToolPreparationInterruption(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "authored deadline"
		if canceled {
			name = "caller canceled"
		}
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
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

			// The budget covers decryption; only caller cancellation escapes as a Go error.
			result, err := f.service.invokeHTTPTool(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`), toolHTTPPolicy())
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

// TestInvokeHTTPToolTimeoutAfterSend retains possible side effects when the
// admission deadline expires after a request reaches the proxy transport.
func TestInvokeHTTPToolTimeoutAfterSend(t *testing.T) {
	for _, name := range []string{"policy deadline", "caller cancellation"} {
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			f.definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(1000))}
			expectHTTPToolDefinition(t, f)
			expectHTTPToolPreparation(f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			expectHTTPToolTransport(t, f, nil, func(r *http.Request) (*http.Response, error) {
				calls++
				if name == "caller cancellation" {
					cancel()
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})

			// Only the policy deadline yields a result; neither interruption retries the call.
			result, err := f.service.invokeHTTPTool(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`), toolHTTPPolicy())
			require.Equal(t, 1, calls)
			if name == "caller cancellation" {
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				requireHTTPToolFailure(t, f, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
			}
		})
	}
}

// TestInvokeHTTPToolInvalidContextAndPolicy rejects invalid internal inputs
// without decrypting connection configuration or creating an HTTP client.
func TestInvokeHTTPToolInvalidContextAndPolicy(t *testing.T) {
	for _, name := range []string{"nil context", "already canceled", "zero timeout", "zero result limit", "invalid authored cap"} {
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			ctx, policy := context.Background(), toolHTTPPolicy()
			switch name {
			case "nil context":
				ctx = nil
			case "already canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "zero timeout":
				policy.HTTP.Timeout = 0
			case "zero result limit":
				policy.Response.MaxEncodedResultBytes = 0
			case "invalid authored cap":
				f.definition.Limits = &toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(0))}
			}
			if name != "nil context" && name != "already canceled" {
				expectHTTPToolDefinition(t, f)
			}

			// Missing or canceled contexts cannot read the snapshot; bad policy stops after authorization.
			result, err := f.service.invokeHTTPTool(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`), policy)
			require.Error(t, err)
			require.Nil(t, result)
			if name == "already canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else if name == "nil context" {
				require.ErrorIs(t, err, ErrInvalidArgument)
			}
		})
	}
}
