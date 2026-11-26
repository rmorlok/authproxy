package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/httpf"
	httpmock "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// javaScriptToolFixture replaces the fixture's HTTP executor while retaining
// the same captured identities, configured connection, aliases, and strict mocks.
func javaScriptToolFixture(t *testing.T, source string) *toolAuthorizationFixture {
	t.Helper()
	f, _ := toolPreparationFixture(t)
	f.definition.ProxyHTTP = nil
	f.definition.Javascript = util.ToPtr(source)
	return f
}

// javaScriptConnectorDefinition serializes a captured helper library alongside
// the no-auth method used by the real configured-client path in these tests.
func javaScriptConnectorDefinition(t *testing.T, source string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"auth": map[string]string{"type": "no-auth"}, "javascript": source,
	})
	require.NoError(t, err)
	return string(encoded)
}

// expectJavaScriptToolPreparation permits only the exact pinned connector and
// configuration ciphertext; any later library lookup must reuse its hydrated definition.
func expectJavaScriptToolPreparation(t *testing.T, f *toolAuthorizationFixture, library, configuration string) {
	t.Helper()
	f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
		Return(javaScriptConnectorDefinition(t, library), nil)
	f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
		Return(configuration, nil)
}

// TestInvokeJavaScriptToolCapturedExecution carries one authorized capture into
// connector helpers, native metadata, per-call middleware, and a complete rich result.
func TestInvokeJavaScriptToolCapturedExecution(t *testing.T) {
	f := javaScriptToolFixture(t, `async function execute(params, context) {
		const [connection] = context.connections;
		const path = customerPath(params.id, connection.cfg.tenant);
		const customer = await (await connection.fetch(path, {labels:{phase:"lookup"}})).json();
		await connection.fetch("https://example.test/audit", {method:"POST", body:"saved", labels:{phase:"audit"}});
		return toolResult({structuredContent:{name:customer.name, generation:helperGeneration,
			connectionCount:context.connections.length, direct:typeof fetch,
			owner:connection.labels.owner, note:connection.annotations.note,
			aliasTenant:cfg.tenant, aliasOwner:labels.owner, aliasNote:annotations.note},
			content:[{type:"text",text:"Customer found"},{type:"image",data:"aGk=",mimeType:"image/png"}],
			provider:{requestId:"req-captured"}});
	}`)
	f.definition.InputSchema = common.RawJSON(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
	f.definition.OutputSchema = common.RawJSON(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	f.snapshot.Connection.Labels = database.Labels{"owner": "captured-owner"}
	f.snapshot.Connection.Annotations = database.Annotations{"note": "captured-note"}
	auth := toolInvocationAuth(toolInvocationGrant("tool:read"), toolInvocationGrant("tool:read"))
	auth.GetActor().Labels = map[string]string{"team": "captured-team"}
	expectHTTPToolDefinition(t, f)
	library := `const helperGeneration = 4;
		function customerPath(id, tenant) { return "https://example.test/" + encodeURIComponent(tenant) + "/" + encodeURIComponent(id); }`
	var preparationDeadline time.Time
	f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
		DoAndReturn(func(ctx context.Context, _ encfield.EncryptedField) (string, error) {
			// Attribution was detached at authorization, before connector hydration.
			auth.GetActor().Labels["team"] = "changed-team"
			var ok bool
			preparationDeadline, ok = ctx.Deadline()
			require.True(t, ok)
			return javaScriptConnectorDefinition(t, library), nil
		})
	f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
		DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
			// The hydrated connector and copied connection metadata must survive later row edits.
			f.snapshot.Connector.Generation = 99
			f.snapshot.Connector.EncryptedDefinition.Data = "new-generation-definition"
			f.snapshot.Connection.Labels["owner"] = "changed-owner"
			f.snapshot.Connection.Annotations["note"] = "changed-note"
			*f.definition.Javascript = `async function execute() { return "changed script"; }`
			return `{"tenant":"acme/team"}`, nil
		})

	// Capture attribution once; each request's labels choose a separate middleware factory.
	factory := f.service.httpf.(*httpmock.MockF)
	factory.EXPECT().ForRequestType(httpf.RequestTypeProxy).Return(factory)
	factory.EXPECT().ForConnection(gomock.Any()).DoAndReturn(func(c httpf.Connection) httpf.F {
		require.Equal(t, f.snapshot.Connection.Id, c.GetId())
		require.Equal(t, uint64(4), c.GetConnectorGeneration())
		require.Equal(t, "captured-owner", c.GetLabels()["owner"])
		return factory
	})
	factory.EXPECT().ForActor(gomock.Any()).DoAndReturn(func(a httpf.Actor) httpf.F {
		require.Equal(t, "captured-team", a.GetLabels()["team"])
		return factory
	})
	factory.EXPECT().ForLabels(gomock.Nil()).Return(factory)
	for _, phase := range []string{"lookup", "audit"} {
		derived := httpmock.NewMockF(gomock.NewController(t))
		factory.EXPECT().ForLabels(map[string]string{"phase": phase}).Return(derived)
		derived.EXPECT().NewHTTPClient().Return(&http.Client{Transport: toolHTTPTransport(func(request *http.Request) (*http.Response, error) {
			deadline, ok := request.Context().Deadline()
			require.True(t, ok)
			require.Equal(t, preparationDeadline, deadline)
			if phase == "lookup" {
				require.Equal(t, "https://example.test/acme%2Fteam/customer%2F42", request.URL.String())
				require.Equal(t, http.MethodGet, request.Method)
			} else {
				require.Equal(t, "https://example.test/audit", request.URL.String())
				require.Equal(t, http.MethodPost, request.Method)
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				require.Equal(t, "saved", string(body))
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{"name":"Ada"}`))}, nil
		})})
	}

	// Matching Tool-only actor/JWT grants execute without proxy permission or any reread.
	require.False(t, auth.Allows(f.snapshot.Connection.Namespace, "connections", "proxy", f.snapshot.Connection.Id.String()))
	got, err := f.service.invokeTool(context.Background(), auth, f.snapshot.Revision.ToolId, util.ToPtr(uint64(7)), common.RawJSON(`{"id":"customer/42"}`))
	require.NoError(t, err)
	require.NoError(t, got.Validate(nil))
	require.False(t, got.IsError)
	require.Equal(t, f.snapshot.Revision.ToolId.String(), got.ToolID)
	require.Equal(t, uint64(7), got.Revision)
	require.Equal(t, f.snapshot.Connection.Id.String(), got.Metadata.ConnectionID)
	require.Equal(t, uint64(3), *got.Metadata.ToolSetGeneration)
	require.JSONEq(t, `{"name":"Ada","generation":4,"connectionCount":1,"direct":"undefined","owner":"captured-owner","note":"captured-note","aliasTenant":"acme/team","aliasOwner":"captured-owner","aliasNote":"captured-note"}`, string(got.StructuredContent))
	require.Len(t, got.Content, 2)
	require.Equal(t, "Customer found", *got.Content[0].Text)
	require.Equal(t, api.ToolContentTypeImage, got.Content[1].Type)
	require.Equal(t, "aGk=", *got.Content[1].Data)
	require.JSONEq(t, `{"requestId":"req-captured"}`, string(got.Provider))
}

// TestInvokeJavaScriptToolPreSendFailures preserves authorization privacy and
// keeps denied, stale, invalid, or unavailable invocations away from HTTP setup.
func TestInvokeJavaScriptToolPreSendFailures(t *testing.T) {
	for _, name := range []string{"denied", "disjoint aliases", "stale revision", "unavailable", "unsupported auth", "invalid arguments", "input schema", "input cap", "source cap", "whitespace library cap", "malformed library", "malformed Tool", "invalid schema", "envelope cap"} {
		t.Run(name, func(t *testing.T) {
			f := javaScriptToolFixture(t, `async function execute() { await context.connections[0].fetch("https://example.test/never"); return true; }`)
			auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
			input, code, library := common.RawJSON(`{}`), "", ""
			var expected *uint64
			var want error
			prepare, compileFailure := true, false
			switch name {
			case "denied":
				auth = toolInvocationAuth(toolInvocationGrant("tool:other"), nil)
				expected, want, prepare = util.ToPtr(uint64(99)), ErrNotFound, false
				f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: &sconfig.ToolInvocationLimits{MaxPendingOperations: util.ToPtr(0)}}})
			case "disjoint aliases":
				f.definition.Verbs = []string{"tool:read", "tool:lookup"}
				auth = toolInvocationAuth(toolInvocationGrant("tool:read"), toolInvocationGrant("tool:lookup"))
				want, prepare = ErrNotFound, false
			case "stale revision":
				expected, want, prepare = util.ToPtr(uint64(99)), ErrToolRevisionChanged, false
			case "unavailable", "envelope cap":
				f.snapshot.Connection.State = database.ConnectionStateDisabled
				code, prepare = "CONNECTION_UNAVAILABLE", false
				if name == "envelope cap" {
					f.definition.Limits = &toolschema.ExecutionLimits{MaxEncodedResultBytes: util.ToPtr(int64(8))}
					want = result.ErrEncodedResultLimit
				}
			case "unsupported auth":
				f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).Return(`{}`, nil)
				code, prepare = "UNSUPPORTED_CAPABILITY", false
			case "invalid arguments":
				input, code = common.RawJSON(`{"private-argument":`), "INPUT_INVALID"
			case "input schema":
				f.definition.InputSchema = common.RawJSON(`{"type":"object","required":["private-field"]}`)
				code = "INPUT_INVALID"
			case "input cap":
				f.definition.Limits = &toolschema.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(2))}
				input, code = common.RawJSON(`{} `), "EXECUTION_LIMIT_EXCEEDED"
			case "source cap", "whitespace library cap":
				maxSource := len(*f.definition.Javascript) - 1
				if name == "whitespace library cap" {
					library = strings.Repeat(" ", 100)
					maxSource = len(*f.definition.Javascript) + 99
				}
				f.service.cfg = config.FromRoot(&sconfig.Root{Tools: &sconfig.Tools{Invocation: &sconfig.ToolInvocationLimits{
					MaxJavascriptSourceBytes: testToolPolicyBytes(t, strconv.Itoa(maxSource)+"b"),
				}}})
				code = "EXECUTION_LIMIT_EXCEEDED"
			case "malformed library":
				library, compileFailure = "function privateBroken( {", true
			case "malformed Tool":
				f.definition.Javascript, compileFailure = util.ToPtr("async function privateBroken( {"), true
			case "invalid schema":
				f.definition.InputSchema, compileFailure = common.RawJSON(`{"type":"private-invalid"}`), true
			}
			expectHTTPToolDefinition(t, f)
			if prepare {
				expectJavaScriptToolPreparation(t, f, library, `{}`)
			}

			// Strict mocks permit no credential or middleware setup before a valid execution.
			got, err := f.service.invokeTool(context.Background(), auth, f.snapshot.Revision.ToolId, expected, input)
			if want != nil || compileFailure {
				require.Error(t, err)
				require.Nil(t, got)
				if want != nil {
					require.ErrorIs(t, err, want)
				}
			} else {
				require.NoError(t, err)
				requireHTTPToolFailure(t, f, got, code, api.ToolExecutionStateNotStarted)
			}
		})
	}
}

// TestInvokeJavaScriptToolInitializationNoIO rejects entry-point and helper
// initialization effects before any configured HTTP client reaches middleware.
func TestInvokeJavaScriptToolInitializationNoIO(t *testing.T) {
	for _, stage := range []string{"Tool", "library"} {
		t.Run(stage, func(t *testing.T) {
			initialization := `context.connections[0].fetch("https://example.test/never");`
			source, library := `async function execute() { return true; }`, ""
			if stage == "Tool" {
				source = initialization + source
			} else {
				library = initialization
			}
			f := javaScriptToolFixture(t, source)
			expectHTTPToolDefinition(t, f)
			expectJavaScriptToolPreparation(t, f, library, `{}`)

			// Source compiles, but initialization cannot acquire an outbound capability.
			got, err := f.service.invokeTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
			require.NoError(t, err)
			requireHTTPToolFailure(t, f, got, "OUTPUT_INVALID", api.ToolExecutionStateNotStarted)
		})
	}
}

// TestInvokeJavaScriptToolPostSendOutcomes maps schema and authored failures
// after a real configured-client call without losing its possible side effects.
func TestInvokeJavaScriptToolPostSendOutcomes(t *testing.T) {
	for _, scenario := range []string{"output schema", "ToolError", "transport"} {
		t.Run(scenario, func(t *testing.T) {
			body := `return {saved:true};`
			if scenario == "ToolError" {
				body = `throw new ToolError("CUSTOMER_REJECTED", "Customer cannot be changed", {reason:"locked"});`
			}
			f := javaScriptToolFixture(t, `async function execute(params, context) {
				await context.connections[0].fetch("https://example.test/update", {method:"POST"}); `+body+` }`)
			f.definition.OutputSchema = common.RawJSON(`false`)
			expectHTTPToolDefinition(t, f)
			expectJavaScriptToolPreparation(t, f, "", `{}`)
			expectHTTPToolTransport(t, f, nil, func(*http.Request) (*http.Response, error) {
				if scenario == "transport" {
					return nil, errors.New("private transport detail")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
			})

			// Explicit errors skip success schemas; all outcomes retain trusted send evidence.
			got, err := f.service.invokeTool(context.Background(), toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil, common.RawJSON(`{}`))
			require.NoError(t, err)
			code := "OUTPUT_INVALID"
			if scenario == "ToolError" {
				code = "CUSTOMER_REJECTED"
			} else if scenario == "transport" {
				code = "UPSTREAM_FAILED"
			}
			requireHTTPToolFailure(t, f, got, code, api.ToolExecutionStateMayHaveExecuted)
			require.False(t, got.Error.Retryable)
			if scenario == "ToolError" {
				require.JSONEq(t, `{"reason":"locked"}`, string(got.Error.Details))
			} else {
				require.Nil(t, got.Error.Details)
			}
		})
	}
}
