package jsexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/render"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// executeLimits leaves room for ordinary Tool results while retaining finite
// bounds for every compilation, validation, VM, HTTP, and envelope phase.
func executeLimits() Limits {
	limits := policyLimits()
	limits.Runtime.MaxSourceBytes = 8192
	limits.Runtime.MaxOutputBytes = 16384
	limits.Validation.MaxOutputBytes = 16384
	return limits
}

// executeDefinition allows object arguments and any successful output unless a
// scenario adds a specific output schema before compilation.
func executeDefinition(source string) toolschema.ToolDefinition {
	return toolschema.ToolDefinition{
		Javascript: util.ToPtr(source), InputSchema: common.RawJSON(`{"type":"object"}`),
	}
}

// executeInvocation represents admission's captured Tool and connection context.
func executeInvocation() Invocation {
	return Invocation{
		ID: "tiv_01example0000001", Arguments: common.RawJSON(`{}`),
		Snapshot: render.Input{
			ToolID: "tol_01example0000001", Revision: 7,
			ConnectionID: "cxn_01example0000001", ConnectionNamespace: "root.product.owner",
		},
	}
}

// executeClient supplies a configured-client seam without actual network I/O.
type executeClient func(*http.Request) (*http.Response, error)

// Do dispatches the request to this test's controlled connection behavior.
func (c executeClient) Do(request *http.Request) (*http.Response, error) { return c(request) }

// DoWithLabels rejects labels unless a scenario explicitly uses a label-aware client.
func (c executeClient) DoWithLabels(request *http.Request, labels map[string]string) (*http.Response, error) {
	if len(labels) != 0 {
		return nil, errors.New("unexpected request labels")
	}
	return c.Do(request)
}

// executeJSONResponse returns a stream owned and closed by the HTTP adapter.
func executeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

// compileExecute prepares one script using the ordinary argument contract.
func compileExecute(t *testing.T, source string, limits Limits) *Program {
	t.Helper()
	program, err := Compile(executeDefinition(source), nil, limits)
	require.NoError(t, err)
	return program
}

// requireExecuteFailure checks public failure shape and preserved evidence while
// rejecting partial success/provider output and automatic retry advice.
func requireExecuteFailure(t *testing.T, result *api.ToolResult, code string, state api.ToolExecutionState) {
	t.Helper()
	require.NotNil(t, result)
	require.NoError(t, result.Validate(nil))
	require.True(t, result.IsError)
	require.NotNil(t, result.Error)
	require.Equal(t, code, result.Error.Code)
	require.Equal(t, state, result.Error.ExecutionState)
	require.False(t, result.Error.Retryable)
	require.Nil(t, result.StructuredContent)
	require.Empty(t, result.Content)
	require.NotNil(t, result.Content)
	require.Nil(t, result.Provider)
}

// TestExecuteSuccessResults preserves ordinary JSON, string display content,
// explicit null, and complete rich payloads in the captured result envelope.
func TestExecuteSuccessResults(t *testing.T) {
	for _, test := range []struct {
		name       string
		value      string
		structured common.RawJSON
		content    []api.ToolContentBlock
		provider   common.RawJSON
	}{
		{"object", `{saved: true}`, common.RawJSON(`{"saved":true}`), []api.ToolContentBlock{}, nil},
		{"string", `"Saved customer"`, common.RawJSON(`"Saved customer"`), []api.ToolContentBlock{{Type: api.ToolContentTypeText, Text: util.ToPtr("Saved customer")}}, nil},
		{"null", `null`, common.RawJSON(`null`), []api.ToolContentBlock{}, nil},
		{"rich", `toolResult({structuredContent: {saved:true}, content:[{type:"text",text:"Saved"},{type:"image",data:"aGk=",mimeType:"image/png"}],provider:{requestId:"req-42"}})`,
			common.RawJSON(`{"saved":true}`), []api.ToolContentBlock{
				{Type: api.ToolContentTypeText, Text: util.ToPtr("Saved")},
				{Type: api.ToolContentTypeImage, Data: util.ToPtr("aGk="), MIMEType: "image/png"},
			}, common.RawJSON(`{"requestId":"req-42"}`)},
		{"rich without structured data", `toolResult({content:[{type:"text",text:"Saved"}]})`, nil,
			[]api.ToolContentBlock{{Type: api.ToolContentTypeText, Text: util.ToPtr("Saved")}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The supplied client is available, but this script needs no requests.
			var calls atomic.Int32
			client := executeClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("unexpected HTTP request")
			})
			program := compileExecute(t, `async function execute() { return `+test.value+`; }`, executeLimits())
			invocation := executeInvocation()
			result, err := program.Execute(context.Background(), client, invocation)

			// Result assembly keeps the exact payload shape and trusted identities.
			require.NoError(t, err)
			require.NoError(t, result.Validate(nil))
			require.False(t, result.IsError)
			require.Equal(t, test.structured, result.StructuredContent)
			require.Equal(t, test.content, result.Content)
			require.Equal(t, test.provider, result.Provider)
			require.Equal(t, invocation.ID, result.InvocationID)
			require.Equal(t, invocation.Snapshot.ToolID, result.ToolID)
			require.Equal(t, invocation.Snapshot.Revision, result.Revision)
			require.Equal(t, invocation.Snapshot.ConnectionID, result.Metadata.ConnectionID)
			require.Zero(t, calls.Load())
		})
	}
}

// TestExecuteToolErrorSkipsOutputSchema retains explicitly authored details and
// obtains failure state from HTTP evidence, even with an impossible output schema.
func TestExecuteToolErrorSkipsOutputSchema(t *testing.T) {
	for _, afterRequest := range []bool{false, true} {
		name := "before request"
		if afterRequest {
			name = "after request"
		}
		t.Run(name, func(t *testing.T) {
			source := `async function execute(params, context) { `
			if afterRequest {
				source += `await context.connections[0].fetch("https://example.test/customer"); `
			}
			source += `throw new ToolError("CUSTOMER_MISSING", "Customer was not found.", {status:404}); }`
			definition := executeDefinition(source)
			definition.OutputSchema = common.RawJSON(`false`)
			program, err := Compile(definition, nil, executeLimits())
			require.NoError(t, err)
			var calls atomic.Int32
			client := executeClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				response := executeJSONResponse(`{"private":"upstream body"}`)
				response.StatusCode = http.StatusNotFound
				return response, nil
			})

			// A non-2xx response remains available to authored JavaScript policy.
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			state := api.ToolExecutionStateNotStarted
			if afterRequest {
				state = api.ToolExecutionStateMayHaveExecuted
				require.Equal(t, int32(1), calls.Load())
			} else {
				require.Zero(t, calls.Load())
			}
			requireExecuteFailure(t, result, "CUSTOMER_MISSING", state)
			require.JSONEq(t, `{"status":404}`, string(result.Error.Details))
			require.Equal(t, "Customer was not found.", result.Error.Message)
		})
	}
}

// TestExecuteOpaqueFailures keeps ordinary exceptions, unsupported returns, and
// uncaught transport failures private while preserving their execution evidence.
func TestExecuteOpaqueFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		calls   int32
		state   api.ToolExecutionState
		success bool
		code    string
	}{
		{"ordinary throw", `throw new Error("private thrown text");`, 0, api.ToolExecutionStateNotStarted, false, "OUTPUT_INVALID"},
		{"undefined return", `const detail = "private source text"; return undefined;`, 0, api.ToolExecutionStateNotStarted, false, "OUTPUT_INVALID"},
		{"function return", `return function() { return "private function text"; };`, 0, api.ToolExecutionStateNotStarted, false, "OUTPUT_INVALID"},
		{"uncaught transport", `await context.connections[0].fetch("https://example.test/private-path"); return true;`, 1, api.ToolExecutionStateMayHaveExecuted, false, "UPSTREAM_FAILED"},
		{"caught transport", `try { await context.connections[0].fetch("https://example.test/private-path"); } catch (_) { return {recovered:true}; } return false;`, 1, api.ToolExecutionStateMayHaveExecuted, true, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The configured client reports a private diagnostic without known-unsent evidence.
			var calls atomic.Int32
			client := executeClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("private transport detail")
			})
			program := compileExecute(t, `async function execute(params, context) { `+test.body+` }`, executeLimits())
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			require.Equal(t, test.calls, calls.Load())
			if test.success {
				// An ordinary transport rejection remains catchable by authored code.
				require.False(t, result.IsError)
				require.JSONEq(t, `{"recovered":true}`, string(result.StructuredContent))
			} else {
				requireExecuteFailure(t, result, test.code, test.state)
			}

			// Neither source text, thrown values, transport diagnostics, nor URLs escape.
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private")
		})
	}
}

// TestExecuteValidatesArgumentsBeforeCode prevents invalid arguments from reaching
// authored initialization or HTTP and makes validated arguments the sole params source.
func TestExecuteValidatesArgumentsBeforeCode(t *testing.T) {
	definition := compiledDefinition(`async function execute(params, context) {
		await context.connections[0].fetch("https://example.test/customer");
		return {saved:true, id:params.id};
	}`)
	program, err := Compile(definition, nil, executeLimits())
	require.NoError(t, err)
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return executeJSONResponse(`{}`), nil
	})
	for _, arguments := range []string{`{}`, `{"id":42}`, `[]`, `{"id":`} {
		// Input rejection occurs before any connection request can begin.
		invocation := executeInvocation()
		invocation.Arguments = common.RawJSON(arguments)
		result, err := program.Execute(context.Background(), client, invocation)
		require.NoError(t, err)
		requireExecuteFailure(t, result, "INPUT_INVALID", api.ToolExecutionStateNotStarted)
		require.Zero(t, calls.Load())
	}

	// A stale or non-JSON Params field is ignored in favor of validated raw arguments.
	invocation := executeInvocation()
	invocation.Arguments = common.RawJSON(`{"id":"caller-42"}`)
	invocation.Snapshot.Params = map[string]any{"id": "stale", "unused": func() {}}
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"saved":true,"id":"caller-42"}`, string(result.StructuredContent))
	require.Equal(t, int32(1), calls.Load())
}

// executeLabelClient records detached request facts from the configured-client
// seam so assertions remain on the test goroutine after evaluation completes.
type executeLabelClient struct {
	mu     sync.Mutex
	urls   []string
	labels []map[string]string
	bodies []string
}

// Do shares the recording path for calls without label overrides.
func (c *executeLabelClient) Do(request *http.Request) (*http.Response, error) {
	return c.DoWithLabels(request, nil)
}

// DoWithLabels records the request and returns the next logical endpoint's JSON.
func (c *executeLabelClient) DoWithLabels(request *http.Request, labels map[string]string) (*http.Response, error) {
	var body []byte
	if request.Body != nil {
		var err error
		body, err = io.ReadAll(request.Body)
		_ = request.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.urls = append(c.urls, request.URL.String())
	c.labels = append(c.labels, maps.Clone(labels))
	c.bodies = append(c.bodies, string(body))
	return executeJSONResponse(`{"id":"customer-42"}`), nil
}

// TestExecuteCapturedConnectionAndHelper composes a multi-request action with
// captured metadata, helper code, input aliases, and per-request labels.
func TestExecuteCapturedConnectionAndHelper(t *testing.T) {
	definition := executeDefinition(`async function execute(params, context) {
		const [connection] = context.connections;
		const response = await connection.fetch(new URL(customerPath(params.id), cfg.apiBase).href);
		const customer = await response.json();
		await connection.fetch(cfg.apiBase + "/audit", {
			method:"POST", body:JSON.stringify({id:customer.id}), labels:{operation:"audit"}
		});
		return {id:customer.id, tenant:connection.cfg.tenant, team:connection.labels.team,
			note:connection.annotations.note, name:connection.name, namespace:connection.namespace,
			connectionId:connection.id, toolId:context.tool.id, revision:context.tool.revision,
			count:context.connections.length, directFetch:typeof fetch,
			aliases:cfg === connection.cfg && labels === connection.labels && annotations === connection.annotations};
	}`)
	library := compiledLibrary(t, `function customerPath(id) { return "/customers/" + encodeURIComponent(id); }`)
	program, err := Compile(definition, library, executeLimits())
	require.NoError(t, err)
	invocation := executeInvocation()
	invocation.Arguments = common.RawJSON(`{"id":"sales/42"}`)
	invocation.Snapshot.ConnectionName = "CRM"
	invocation.Snapshot.Cfg = map[string]any{"apiBase": "https://example.test", "tenant": "acme"}
	invocation.Snapshot.Labels = map[string]string{"team": "blue"}
	invocation.Snapshot.Annotations = map[string]string{"note": "captured"}
	client := &executeLabelClient{}

	// Both logical requests use the same configured connection; direct fetch is absent.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"id":"customer-42","tenant":"acme","team":"blue","note":"captured","name":"CRM","namespace":"root.product.owner","connectionId":"cxn_01example0000001","toolId":"tol_01example0000001","revision":7,"count":1,"directFetch":"undefined","aliases":true}`, string(result.StructuredContent))
	client.mu.Lock()
	defer client.mu.Unlock()
	require.Equal(t, []string{"https://example.test/customers/sales%2F42", "https://example.test/audit"}, client.urls)
	require.Equal(t, []map[string]string{nil, {"operation": "audit"}}, client.labels)
	require.JSONEq(t, `{"id":"customer-42"}`, client.bodies[1])
}

// TestExecuteOutputSchemaFailureRetainsSendEvidence rejects mismatched successful
// output after I/O without returning partial data or implying the action was unsent.
func TestExecuteOutputSchemaFailureRetainsSendEvidence(t *testing.T) {
	definition := compiledDefinition(`async function execute(params, context) {
		await context.connections[0].fetch("https://example.test/customer");
		return toolResult({structuredContent:{saved:"wrong"},content:[{type:"text",text:"private partial"}],provider:{requestId:"private"}});
	}`)
	program, err := Compile(definition, nil, executeLimits())
	require.NoError(t, err)
	invocation := executeInvocation()
	invocation.Arguments = common.RawJSON(`{"id":"customer-42"}`)
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return executeJSONResponse(`{}`), nil
	})

	// Schema validation runs after successful JavaScript conversion and session cleanup.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	requireExecuteFailure(t, result, "OUTPUT_INVALID", api.ToolExecutionStateMayHaveExecuted)
	require.Equal(t, int32(1), calls.Load())
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private")
}

// TestExecuteEnvelopeFallbackClearsPayload covers provider metadata and explicit
// error details that fit runtime conversion but exceed the complete result cap.
func TestExecuteEnvelopeFallbackClearsPayload(t *testing.T) {
	for _, source := range []string{
		`async function execute() { return toolResult({structuredContent:{saved:true},content:[{type:"text",text:"partial"}],provider:{data:"x".repeat(2048)}}); }`,
		`async function execute() { throw new ToolError("CUSTOMER_FAILED", "Customer failed.", {data:"x".repeat(2048)}); }`,
	} {
		// A modest envelope cap still leaves room for the platform's compact fallback.
		limits := executeLimits()
		limits.MaxEncodedResultBytes = 512
		program := compileExecute(t, source, limits)
		client := executeClient(func(*http.Request) (*http.Response, error) { return nil, errors.New("unexpected HTTP") })
		result, err := program.Execute(context.Background(), client, executeInvocation())
		require.NoError(t, err)
		requireExecuteFailure(t, result, "OUTPUT_TOO_LARGE", api.ToolExecutionStateNotStarted)
		require.Nil(t, result.Error.Details)
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		require.LessOrEqual(t, len(encoded), limits.MaxEncodedResultBytes)

		// An envelope too small for any safe failure returns no partial result.
		limits.MaxEncodedResultBytes = 128
		program = compileExecute(t, source, limits)
		result, err = program.Execute(context.Background(), client, executeInvocation())
		require.ErrorIs(t, err, toolresult.ErrEncodedResultLimit)
		require.Nil(t, result)
	}
}

// TestExecuteConcurrentOwnership isolates VM mutations, provenance pointers, and
// returned payload bytes across concurrent uses of one compiled program.
func TestExecuteConcurrentOwnership(t *testing.T) {
	program := compileExecute(t, `let calls = 0; async function execute(params, context) {
		calls++; cfg.nested.value++; labels.team = "local";
		return {calls, value:cfg.nested.value, id:params.id};
	}`, executeLimits())
	invocation := executeInvocation()
	invocation.Arguments = common.RawJSON(`{"id":"customer-42"}`)
	invocation.Snapshot.Cfg = map[string]any{"nested": map[string]any{"value": 1}}
	invocation.Snapshot.Labels = map[string]string{"team": "blue"}
	invocation.Snapshot.ToolSetGeneration = util.ToPtr(uint64(3))
	invocation.Snapshot.CatalogRevision = util.ToPtr(uint64(8))
	client := executeClient(func(*http.Request) (*http.Response, error) { return nil, errors.New("unexpected HTTP") })
	type outcome struct {
		result *api.ToolResult
		err    error
	}
	outcomes := make(chan outcome, 8)
	for range 8 {
		go func() {
			result, err := program.Execute(context.Background(), client, invocation)
			outcomes <- outcome{result, err}
		}()
	}

	// Each invocation gets fresh global and configuration state.
	results := make([]*api.ToolResult, 0, 8)
	for range 8 {
		outcome := <-outcomes
		require.NoError(t, outcome.err)
		require.False(t, outcome.result.IsError)
		require.JSONEq(t, `{"calls":1,"value":2,"id":"customer-42"}`, string(outcome.result.StructuredContent))
		results = append(results, outcome.result)
	}
	require.Equal(t, 1, invocation.Snapshot.Cfg["nested"].(map[string]any)["value"])
	require.Equal(t, "blue", invocation.Snapshot.Labels["team"])

	// Neither changing source provenance nor one result changes another result.
	*invocation.Snapshot.ToolSetGeneration = 99
	*invocation.Snapshot.CatalogRevision = 100
	results[0].StructuredContent[0] = ' '
	*results[0].Metadata.ToolSetGeneration = 101
	for _, result := range results[1:] {
		require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
		require.Equal(t, uint64(8), *result.Metadata.CatalogRevision)
		require.JSONEq(t, `{"calls":1,"value":2,"id":"customer-42"}`, string(result.StructuredContent))
	}
}
