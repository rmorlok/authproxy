package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
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

// testToolPolicyBytes constructs the same byte-size value that configuration decoding supplies.
func testToolPolicyBytes(t *testing.T, size string) *sconfig.HumanByteSize {
	t.Helper()
	encoded, err := json.Marshal(size)
	require.NoError(t, err)
	var value sconfig.HumanByteSize
	require.NoError(t, json.Unmarshal(encoded, &value))
	return &value
}

// TestToolHTTPInvocationPolicyDefaults supports partial service configurations
// and supplies a complete policy, including the optional response expression runtime.
func TestToolHTTPInvocationPolicyDefaults(t *testing.T) {
	want := httpexec.Limits{
		Render: render.Limits{MaxBytes: 8 << 20, MaxDepth: 64, MaxSteps: 1_000_000},
		Validation: validation.Limits{MaxInputBytes: 1 << 20, MaxOutputBytes: 8 << 20,
			MaxDepth: 64, MaxValues: 1_000_000, MaxNumberDigits: 4096},
		Response: httpcodec.ResponseLimits{MaxSourceBytes: 256 << 10, MaxResponseBytes: 8 << 20,
			MaxOutputBytes: 8 << 20, MaxEncodedResultBytes: 16 << 20,
			Transform: javascript.Limits{MaxSourceBytes: 256 << 10, MaxInputBytes: 16 << 20,
				MaxOutputBytes: 8 << 20, MaxDepth: 64, MaxNodes: 1_000_000, MaxStackDepth: 256, Timeout: time.Minute}},
		HTTP: httphost.Limits{Timeout: time.Minute, MaxRequests: 20,
			MaxResponseBytes: 8 << 20, MaxStreamBytes: 8 << 20, MaxHeaderBytes: 64 << 10},
	}
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
			// Every omitted configuration level selects the same documented defaults.
			s := &service{cfg: test.cfg}
			got, err := s.toolHTTPInvocationPolicy()
			require.NoError(t, err)
			require.Equal(t, want, got)

			// A transformed Tool must compile without supplying any hidden runtime limits.
			definition := toolschema.ToolDefinition{
				InputSchema: common.RawJSON(`{"type":"object"}`),
				ProxyHTTP: &toolschema.ProxyHTTP{Method: "GET", URL: "https://example.test",
					Response: &toolschema.HTTPResponse{TransformJavascript: util.ToPtr(`response.body`)}},
			}
			_, err = httpexec.Compile(definition, got)
			require.NoError(t, err)

			// The returned policy is owned by the invocation, not a mutable shared default.
			got.HTTP.MaxRequests = 1
			again, err := s.toolHTTPInvocationPolicy()
			require.NoError(t, err)
			require.Equal(t, want, again)
		})
	}
}

// TestToolHTTPInvocationPolicyConfigured maps independent operator budgets into
// their consumer layers without retaining pointers to mutable configuration.
func TestToolHTTPInvocationPolicyConfigured(t *testing.T) {
	settings := &sconfig.ToolInvocationLimits{
		Timeout: &sconfig.HumanDuration{Duration: 3 * time.Second}, MaxRequests: util.ToPtr(4),
		MaxInputBytes: testToolPolicyBytes(t, "2kib"), MaxResponseBytes: testToolPolicyBytes(t, "3kib"),
		MaxOutputBytes: testToolPolicyBytes(t, "4kib"), MaxEncodedResultBytes: testToolPolicyBytes(t, "5kib"),
		MaxStackDepth: util.ToPtr(48), MaxRenderBytes: testToolPolicyBytes(t, "6kib"),
		MaxTransformInputBytes: testToolPolicyBytes(t, "7kib"), MaxResponseStreamBytes: testToolPolicyBytes(t, "8kib"),
		MaxResponseHeaderBytes: testToolPolicyBytes(t, "9kib"), MaxResponseSourceBytes: testToolPolicyBytes(t, "10kib"),
		MaxDepth: util.ToPtr(12), MaxValues: util.ToPtr(3000), MaxRenderSteps: util.ToPtr(4000), MaxNumberDigits: util.ToPtr(80),
	}
	s := &service{cfg: config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})}
	want := httpexec.Limits{
		Render: render.Limits{MaxBytes: 6 << 10, MaxDepth: 12, MaxSteps: 4000},
		Validation: validation.Limits{MaxInputBytes: 2 << 10, MaxOutputBytes: 4 << 10,
			MaxDepth: 12, MaxValues: 3000, MaxNumberDigits: 80},
		Response: httpcodec.ResponseLimits{MaxSourceBytes: 10 << 10, MaxResponseBytes: 3 << 10,
			MaxOutputBytes: 4 << 10, MaxEncodedResultBytes: 5 << 10,
			Transform: javascript.Limits{MaxSourceBytes: 10 << 10, MaxInputBytes: 7 << 10,
				MaxOutputBytes: 4 << 10, MaxDepth: 12, MaxNodes: 3000, MaxStackDepth: 48, Timeout: 3 * time.Second}},
		HTTP: httphost.Limits{Timeout: 3 * time.Second, MaxRequests: 4,
			MaxResponseBytes: 3 << 10, MaxStreamBytes: 8 << 10, MaxHeaderBytes: 9 << 10},
	}

	// Shared concepts such as output bytes reach each relevant layer; distinct inputs stay independent.
	got, err := s.toolHTTPInvocationPolicy()
	require.NoError(t, err)
	require.Equal(t, want, got)

	// Editing the source configuration later cannot change a previously captured policy.
	settings.Timeout.Duration = time.Hour
	*settings.MaxRequests = 99
	require.NoError(t, json.Unmarshal([]byte(`"99mib"`), settings.MaxInputBytes))
	require.Equal(t, want, got)
}

// TestInvokeHTTPToolConfiguredInputCaps honors the smaller operator/authored cap
// before creating middleware, while allowing authorization errors to take priority.
func TestInvokeHTTPToolConfiguredInputCaps(t *testing.T) {
	for _, authoredIsSmaller := range []bool{false, true} {
		name := "operator cap"
		if authoredIsSmaller {
			name = "authored cap"
		}
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			settings := &sconfig.ToolInvocationLimits{MaxInputBytes: testToolPolicyBytes(t, "2b")}
			f.definition.Limits = &toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(100))}
			if authoredIsSmaller {
				settings.MaxInputBytes = testToolPolicyBytes(t, "100b")
				f.definition.Limits.MaxInputBytes = util.ToPtr(int64(2))
			}
			f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})
			expectHTTPToolDefinition(t, f)
			expectHTTPToolPreparation(f)

			// Even semantically empty whitespace consumes argument bytes; strict mocks reject I/O.
			result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{} `))
			require.NoError(t, err)
			requireHTTPToolFailure(t, f, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
		})
	}

	t.Run("authorization precedes invalid policy", func(t *testing.T) {
		f, _ := toolPreparationFixture(t)
		f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{
			Invocation: &sconfig.ToolInvocationLimits{MaxRequests: util.ToPtr(0)},
		}})
		expectHTTPToolDefinition(t, f)

		// A denied Tool is still hidden even when the operator configuration is invalid.
		result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:other"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, result)
	})
}

// TestInvokeHTTPToolConfiguredResultCaps checks configured limits after a send,
// retaining execution uncertainty while discarding oversized upstream content.
func TestInvokeHTTPToolConfiguredResultCaps(t *testing.T) {
	for _, name := range []string{"response bytes", "output bytes", "encoded envelope", "transform input"} {
		t.Run(name, func(t *testing.T) {
			f, _ := toolPreparationFixture(t)
			settings := &sconfig.ToolInvocationLimits{}
			payload := `{"private-value":"` + strings.Repeat("x", 1024) + `"}`
			code := "OUTPUT_TOO_LARGE"
			switch name {
			case "response bytes":
				settings.MaxResponseBytes = testToolPolicyBytes(t, "8b")
				f.definition.ProxyHTTP.Response = &toolschema.HTTPResponse{TransformJavascript: util.ToPtr(`true`)}
			case "output bytes":
				settings.MaxOutputBytes = testToolPolicyBytes(t, "8b")
			case "encoded envelope":
				settings.MaxEncodedResultBytes = testToolPolicyBytes(t, "600b")
			case "transform input":
				settings.MaxTransformInputBytes = testToolPolicyBytes(t, "8b")
				f.definition.ProxyHTTP.Response = &toolschema.HTTPResponse{TransformJavascript: util.ToPtr(`true`)}
				code = "EXECUTION_LIMIT_EXCEEDED"
			}
			f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})
			expectHTTPToolDefinition(t, f)
			expectHTTPToolPreparation(f)
			calls := 0
			expectHTTPToolTransport(t, f, nil, func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})

			// A shrinking projection cannot bypass the independent upstream/input limits.
			result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			requireHTTPToolFailure(t, f, result, code, api.ToolExecutionStateMayHaveExecuted)
			if settings.MaxEncodedResultBytes != nil {
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				require.LessOrEqual(t, uint64(len(encoded)), settings.MaxEncodedResultBytes.Value())
			}
		})
	}
}

// TestInvokeHTTPToolIndependentInputBudgets keeps a small argument cap from
// consuming the separately configured connection-rendering and projection budgets.
func TestInvokeHTTPToolIndependentInputBudgets(t *testing.T) {
	f, _ := toolPreparationFixture(t)
	settings := &sconfig.ToolInvocationLimits{MaxInputBytes: testToolPolicyBytes(t, "2b")}
	f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})
	f.definition.ProxyHTTP.Headers = map[string]string{"X-Tenant": "{{cfg.tenant}}"}
	f.definition.ProxyHTTP.Response = &toolschema.HTTPResponse{
		TransformJavascript: util.ToPtr(`({tenant: cfg.tenant, value: response.body.value})`),
	}
	expectHTTPToolDefinition(t, f)
	expectHTTPToolPreparation(f)
	expectHTTPToolTransport(t, f, nil, func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "acme/team", request.Header.Get("X-Tenant"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"value":"provider response"}`))}, nil
	})

	// The two-byte argument is valid while the larger connection snapshot and response flow through.
	result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"tenant":"acme/team","value":"provider response"}`, string(result.StructuredContent))
}

// TestInvokeHTTPToolConfiguredRenderCap rejects an oversized connection snapshot
// before client construction, even when the caller's own arguments fit their cap.
func TestInvokeHTTPToolConfiguredRenderCap(t *testing.T) {
	f, _ := toolPreparationFixture(t)
	settings := &sconfig.ToolInvocationLimits{
		MaxInputBytes:  testToolPolicyBytes(t, "2b"),
		MaxRenderBytes: testToolPolicyBytes(t, "512b"),
	}
	f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: settings}})
	f.snapshot.Connection.Labels = map[string]string{"large": strings.Repeat("x", 1024)}
	expectHTTPToolDefinition(t, f)
	expectHTTPToolPreparation(f)

	// Strict factory mocks prove the render allowance is enforced before I/O.
	result, err := f.service.invokeHTTPTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
	require.NoError(t, err)
	requireHTTPToolFailure(t, f, result, "INPUT_INVALID", api.ToolExecutionStateNotStarted)
}
