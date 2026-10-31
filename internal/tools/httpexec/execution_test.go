package httpexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// executionLimits leaves room for normal requests while bounding every layer.
func executionLimits() httpexec.Limits {
	return httpexec.Limits{
		Render: render.Limits{MaxBytes: 32768, MaxDepth: 32, MaxSteps: 10000},
		Validation: validation.Limits{
			MaxInputBytes: 16384, MaxOutputBytes: 16384, MaxDepth: 32,
			MaxValues: 10000, MaxNumberDigits: 128,
		},
		Response: httpcodec.ResponseLimits{
			MaxSourceBytes: 4096, MaxResponseBytes: 16384,
			MaxOutputBytes: 16384, MaxEncodedResultBytes: 32768,
			Transform: javascript.Limits{
				MaxSourceBytes: 4096, MaxInputBytes: 32768, MaxOutputBytes: 16384,
				MaxDepth: 32, MaxNodes: 10000, MaxStackDepth: 128, Timeout: time.Second,
			},
		},
		HTTP: httphost.Limits{
			Timeout: 2 * time.Second, MaxRequests: 1, MaxResponseBytes: 16384,
			MaxStreamBytes: 16384, MaxHeaderBytes: 4096,
		},
	}
}

// executionDefinition provides the smallest ordinary HTTP Tool contract.
func executionDefinition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Description: "Read a contact", Verbs: []string{"read"},
		InputSchema: common.RawJSON(`{"type":"object"}`),
		ProxyHTTP:   &tools.ProxyHTTP{Method: "GET", URL: "https://example.test/contact"},
	}
}

// executionInvocation supplies valid captured identities and caller arguments.
func executionInvocation() httpexec.Invocation {
	return httpexec.Invocation{
		ID: "tiv_01example0000001", Arguments: common.RawJSON(`{}`),
		Snapshot: render.Input{
			ToolID: "tol_01example0000001", Revision: 7,
			ConnectionID: "cxn_01example0000001", ConnectionNamespace: "root.product.owner",
		},
	}
}

// executionClient injects a connection client's behavior without real network I/O.
type executionClient func(*http.Request) (*http.Response, error)

// Do transfers the request to the behavior selected by the current scenario.
func (c executionClient) Do(request *http.Request) (*http.Response, error) { return c(request) }

// executionBody records response ownership and reads, including discarded errors.
type executionBody struct {
	reader io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

// Read records each attempt to consume provider bytes.
func (b *executionBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(p)
}

// Close records each release of the provider stream.
func (b *executionBody) Close() error { b.closes.Add(1); return nil }

// executionResponse wraps ordinary JSON in an observable provider stream.
func executionResponse(body *executionBody) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}
}

// compileExecution requires a reusable program for an authored definition.
func compileExecution(t *testing.T, definition tools.ToolDefinition, limits httpexec.Limits) *httpexec.Program {
	t.Helper()
	program, err := httpexec.Compile(definition, limits)
	require.NoError(t, err)
	return program
}

// requireExecutionFailure checks the safe envelope, absence of partial content,
// and exact execution knowledge without depending on wording of public messages.
func requireExecutionFailure(t *testing.T, result *api.ToolResult, code string, state api.ToolExecutionState) {
	t.Helper()
	require.NotNil(t, result)
	require.NoError(t, result.Validate(nil))
	require.True(t, result.IsError)
	require.Equal(t, code, result.Error.Code)
	require.Equal(t, state, result.Error.ExecutionState)
	require.Empty(t, result.Content)
	require.Nil(t, result.StructuredContent)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
}

// TestExecuteComposesRequestAndResponse verifies the whole data path, including
// exact input numbers, tenant routing, filtered headers, and output validation.
func TestExecuteComposesRequestAndResponse(t *testing.T) {
	definition := executionDefinition()
	definition.InputSchema = common.RawJSON(`{"type":"object","required":["id","amount"],"properties":{"id":{"type":"string"},"amount":{"type":"integer"}},"additionalProperties":false}`)
	definition.OutputSchema = common.RawJSON(`{"type":"object","required":["id","amount","tenant","requestId"],"properties":{"amount":{"const":"9007199254740993"}}}`)
	definition.ProxyHTTP = &tools.ProxyHTTP{
		Method: "POST", URL: "https://{{cfg.host}}/tenants/{{path.cfg.tenant}}/contacts/{{path.params.id}}",
		Headers:  map[string]string{"Host": "{{cfg.override}}", "X-Team": "{{labels.team}}"},
		Query:    map[string]common.RawJSON{"tag": common.RawJSON(`{"$value":"cfg.tags"}`)},
		BodyJSON: common.RawJSON(`{"amount":{"$value":"params.amount"}}`),
		Response: &tools.HTTPResponse{TransformJavascript: util.ToPtr(`({id: params.id, amount: response.body.amount, tenant: context.cfg.tenant, requestId: response.headers["x-request-id"][0], headers: response.headers})`)},
	}
	invocation := executionInvocation()
	invocation.Arguments = common.RawJSON(`{"id":"sales/42","amount":9007199254740993}`)
	invocation.Snapshot.Cfg = map[string]any{
		"host": "api.example.test", "tenant": "acme/team", "override": "tenant.example.test:8443",
		"tags": []any{"lead & web", "new"},
	}
	invocation.Snapshot.Labels = map[string]string{"team": "blue"}
	invocation.Snapshot.ToolSetGeneration = util.ToPtr(uint64(4))
	invocation.Snapshot.CatalogRevision = util.ToPtr(uint64(2))
	// Raw validated arguments replace Params, including values invalid as snapshots.
	invocation.Snapshot.Params = map[string]any{"unused": func() {}}
	program := compileExecution(t, definition, executionLimits())
	body := &executionBody{reader: strings.NewReader(`{"amount":"9007199254740993"}`)}
	client := executionClient(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "POST", request.Method)
		require.Equal(t, "https://api.example.test/tenants/acme%2Fteam/contacts/sales%2F42?tag=lead+%26+web&tag=new", request.URL.String())
		require.Equal(t, "tenant.example.test:8443", request.Host)
		require.Equal(t, "blue", request.Header.Get("X-Team"))
		data, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		require.NoError(t, request.Body.Close())
		require.JSONEq(t, `{"amount":9007199254740993}`, string(data))
		response := executionResponse(body)
		response.Header["X-Request-Id"] = []string{"req-7"}
		response.Header["Set-Cookie"] = []string{"private-cookie"}
		response.Header["Authorization"] = []string{"private-auth"}
		response.Header["Connection"] = []string{"X-Private"}
		response.Header["X-Private"] = []string{"private-nominated"}
		return response, nil
	})

	// A successful call preserves captured source identity and closes its stream.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NoError(t, result.Validate(nil))
	require.Equal(t, invocation.ID, result.InvocationID)
	require.Equal(t, invocation.Snapshot.ToolID, result.ToolID)
	require.Equal(t, invocation.Snapshot.Revision, result.Revision)
	require.Equal(t, invocation.Snapshot.ConnectionID, result.Metadata.ConnectionID)
	require.Equal(t, uint64(4), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(2), *result.Metadata.CatalogRevision)
	require.JSONEq(t, `{"id":"sales/42","amount":"9007199254740993","tenant":"acme/team","requestId":"req-7","headers":{"content-type":["application/json"],"x-request-id":["req-7"]}}`, string(result.StructuredContent))
	require.EqualValues(t, 1, body.closes.Load())
	// Returned provenance does not retain pointers owned by the invocation caller.
	*invocation.Snapshot.ToolSetGeneration = 99
	*invocation.Snapshot.CatalogRevision = 99
	require.Equal(t, uint64(4), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(2), *result.Metadata.CatalogRevision)
}

// TestExecuteRejectsInputBeforeIO ensures no invalid argument or rendered value
// reaches the connection client, and identities are checked before execution.
func TestExecuteRejectsInputBeforeIO(t *testing.T) {
	for _, test := range []struct {
		name     string
		change   func(*tools.ToolDefinition, *httpexec.Invocation, *httpexec.Limits)
		code     string
		internal bool
	}{
		{"malformed JSON", func(_ *tools.ToolDefinition, i *httpexec.Invocation, _ *httpexec.Limits) {
			i.Arguments = common.RawJSON(`{"private-value":`)
		}, "INPUT_INVALID", false},
		{"schema mismatch", func(d *tools.ToolDefinition, _ *httpexec.Invocation, _ *httpexec.Limits) {
			d.InputSchema = common.RawJSON(`{"type":"object","required":["private-field"]}`)
		}, "INPUT_INVALID", false},
		{"missing reference", func(d *tools.ToolDefinition, _ *httpexec.Invocation, _ *httpexec.Limits) {
			d.ProxyHTTP.URL += "/{{params.private_missing}}"
		}, "INPUT_INVALID", false},
		{"invalid rendered header", func(d *tools.ToolDefinition, i *httpexec.Invocation, _ *httpexec.Limits) {
			d.ProxyHTTP.Headers = map[string]string{"X-Value": "{{params.value}}"}
			i.Arguments = common.RawJSON(`{"value":"private-value\r\nInjected: true"}`)
		}, "INPUT_INVALID", false},
		{"input limit", func(_ *tools.ToolDefinition, i *httpexec.Invocation, l *httpexec.Limits) {
			l.Validation.MaxInputBytes = 2
			i.Arguments = common.RawJSON(`{"private-value":true}`)
		}, "EXECUTION_LIMIT_EXCEEDED", false},
		{"invalid identity", func(_ *tools.ToolDefinition, i *httpexec.Invocation, _ *httpexec.Limits) { i.ID = "private-invalid" }, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition, invocation, limits := executionDefinition(), executionInvocation(), executionLimits()
			test.change(&definition, &invocation, &limits)
			program := compileExecution(t, definition, limits)
			var calls atomic.Int32
			client := executionClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("private-unexpected call")
			})
			result, err := program.Execute(context.Background(), client, invocation)
			if test.internal {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				requireExecutionFailure(t, result, test.code, api.ToolExecutionStateNotStarted)
			}
			require.Zero(t, calls.Load())
		})
	}
}

// TestExecuteStatusErrorsDoNotReadBodies applies status policy before body
// decoding and output schema checks, even for explicitly mapped 2xx responses.
func TestExecuteStatusErrorsDoNotReadBodies(t *testing.T) {
	for _, status := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			definition := executionDefinition()
			definition.OutputSchema = common.RawJSON(`false`)
			definition.ProxyHTTP.Response = &tools.HTTPResponse{
				Errors:              []tools.HTTPErrorRule{{Statuses: []int{200, 404}, Code: "CONTACT_UNAVAILABLE", Message: "The contact is unavailable.", Retryable: util.ToPtr(true)}},
				TransformJavascript: util.ToPtr(`(() => { throw new Error("private-transform"); })()`),
			}
			program := compileExecution(t, definition, executionLimits())
			body := &executionBody{reader: strings.NewReader("private-invalid-gzip")}
			var calls atomic.Int32
			client := executionClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				response := executionResponse(body)
				response.StatusCode = status
				response.Header.Set("Content-Encoding", "gzip")
				return response, nil
			})

			// Retry advice is preserved without repeating the operation or reading errors.
			result, err := program.Execute(context.Background(), client, executionInvocation())
			require.NoError(t, err)
			code := "CONTACT_UNAVAILABLE"
			if status == 503 {
				code = "UPSTREAM_FAILED"
			}
			requireExecutionFailure(t, result, code, api.ToolExecutionStateMayHaveExecuted)
			require.Equal(t, status != 503, result.Error.Retryable)
			require.Zero(t, body.reads.Load())
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// TestExecuteTransportFailureEvidence keeps safe credential classification
// independent from proof that a target request was never sent.
func TestExecuteTransportFailureEvidence(t *testing.T) {
	for _, knownUnsent := range []bool{false, true} {
		for _, unavailable := range []bool{false, true} {
			t.Run(fmt.Sprintf("unsent=%t/credentials=%t", knownUnsent, unavailable), func(t *testing.T) {
				program := compileExecution(t, executionDefinition(), executionLimits())
				body := &executionBody{reader: strings.NewReader("private-response")}
				var calls atomic.Int32
				client := executionClient(func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					if knownUnsent {
						httpf.ReportSendEvidence(request.Context(), false)
					}
					cause := errors.New("private-transport URL and credentials")
					if unavailable {
						cause = auth_methods.MarkCredentialsUnavailable(cause)
					}
					return executionResponse(body), cause
				})

				// Even a simultaneous response is discarded and closed without leaking it.
				result, err := program.Execute(context.Background(), client, executionInvocation())
				require.NoError(t, err)
				code, state := "UPSTREAM_FAILED", api.ToolExecutionStateMayHaveExecuted
				if unavailable {
					code = "CONNECTION_UNAVAILABLE"
				}
				if knownUnsent {
					state = api.ToolExecutionStateNotStarted
				}
				requireExecutionFailure(t, result, code, state)
				require.Zero(t, body.reads.Load())
				require.EqualValues(t, 1, body.closes.Load())
				require.EqualValues(t, 1, calls.Load())
			})
		}
	}
}

// failingExecutionReader exposes a transport read failure after partial bytes.
type failingExecutionReader struct{}

// Read returns both private bytes and a private error, neither safe for output.
func (failingExecutionReader) Read(p []byte) (int, error) {
	return copy(p, "private-partial"), errors.New("private-stream failure")
}

// TestExecuteResponseFailures drops partial output, preserves possible effects,
// and releases the provider body on all decoding, validation, and budget failures.
func TestExecuteResponseFailures(t *testing.T) {
	for _, test := range []struct {
		name, code string
		change     func(*tools.ToolDefinition, *httpexec.Limits, *http.Response, *executionBody)
	}{
		{"read failure", "UPSTREAM_FAILED", func(_ *tools.ToolDefinition, _ *httpexec.Limits, _ *http.Response, b *executionBody) {
			b.reader = failingExecutionReader{}
		}},
		{"invalid encoding", "UPSTREAM_PROTOCOL_ERROR", func(_ *tools.ToolDefinition, _ *httpexec.Limits, r *http.Response, _ *executionBody) {
			r.Header.Set("Content-Encoding", "gzip")
		}},
		{"invalid response", "UPSTREAM_PROTOCOL_ERROR", func(_ *tools.ToolDefinition, _ *httpexec.Limits, r *http.Response, _ *executionBody) {
			r.StatusCode = 600
		}},
		{"decoded byte cap", "OUTPUT_TOO_LARGE", func(_ *tools.ToolDefinition, l *httpexec.Limits, _ *http.Response, _ *executionBody) {
			l.HTTP.MaxResponseBytes = 4
		}},
		{"stream byte cap", "OUTPUT_TOO_LARGE", func(_ *tools.ToolDefinition, l *httpexec.Limits, _ *http.Response, _ *executionBody) {
			l.HTTP.MaxStreamBytes = 4
		}},
		{"header byte cap", "OUTPUT_TOO_LARGE", func(_ *tools.ToolDefinition, l *httpexec.Limits, _ *http.Response, _ *executionBody) {
			l.HTTP.MaxHeaderBytes = 4
		}},
		{"malformed output JSON", "OUTPUT_INVALID", func(_ *tools.ToolDefinition, _ *httpexec.Limits, _ *http.Response, b *executionBody) {
			b.reader = strings.NewReader(`{"private-value":`)
		}},
		{"output schema", "OUTPUT_INVALID", func(d *tools.ToolDefinition, _ *httpexec.Limits, r *http.Response, b *executionBody) {
			d.OutputSchema = common.RawJSON(`{"type":"integer"}`)
			r.Header.Set("Content-Type", "text/plain")
			b.reader = strings.NewReader("private-text")
		}},
		{"output validation cap", "OUTPUT_TOO_LARGE", func(d *tools.ToolDefinition, l *httpexec.Limits, _ *http.Response, _ *executionBody) {
			d.OutputSchema = common.RawJSON(`true`)
			l.Validation.MaxOutputBytes = 4
		}},
		{"output validation values", "OUTPUT_TOO_LARGE", func(d *tools.ToolDefinition, l *httpexec.Limits, _ *http.Response, _ *executionBody) {
			d.OutputSchema = common.RawJSON(`true`)
			l.Validation.MaxValues = 1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition, limits := executionDefinition(), executionLimits()
			body := &executionBody{reader: strings.NewReader(`{"ok":true}`)}
			response := executionResponse(body)
			test.change(&definition, &limits, response, body)
			program := compileExecution(t, definition, limits)
			client := executionClient(func(*http.Request) (*http.Response, error) { return response, nil })
			result, err := program.Execute(context.Background(), client, executionInvocation())
			require.NoError(t, err)
			requireExecutionFailure(t, result, test.code, api.ToolExecutionStateMayHaveExecuted)
			require.EqualValues(t, 1, body.closes.Load())
		})
	}
}

// TestExecuteRichContent preserves binary output when no structured schema is
// declared; an explicit schema still requires structured content to be present.
func TestExecuteRichContent(t *testing.T) {
	for _, requireStructured := range []bool{false, true} {
		t.Run(fmt.Sprint(requireStructured), func(t *testing.T) {
			definition := executionDefinition()
			if requireStructured {
				definition.OutputSchema = common.RawJSON(`true`)
			}
			program := compileExecution(t, definition, executionLimits())
			body := &executionBody{reader: strings.NewReader(string([]byte{0, 255, 128}))}
			client := executionClient(func(*http.Request) (*http.Response, error) {
				response := executionResponse(body)
				response.Header.Set("Content-Type", "image/png")
				return response, nil
			})

			// Image bytes stay a rich block and cannot substitute for structured JSON.
			result, err := program.Execute(context.Background(), client, executionInvocation())
			require.NoError(t, err)
			if requireStructured {
				requireExecutionFailure(t, result, "OUTPUT_INVALID", api.ToolExecutionStateMayHaveExecuted)
			} else {
				require.NoError(t, result.Validate(nil))
				require.False(t, result.IsError)
				require.Nil(t, result.StructuredContent)
				require.Len(t, result.Content, 1)
				require.Equal(t, api.ToolContentTypeImage, result.Content[0].Type)
				require.Equal(t, "image/png", result.Content[0].MIMEType)
				require.Equal(t, "AP+A", *result.Content[0].Data)
			}
			require.EqualValues(t, 1, body.closes.Load())
		})
	}
}

// TestExecuteCancellationAndTimeout distinguishes the caller's context from the
// whole-invocation timeout while keeping the program reusable after interruption.
func TestExecuteCancellationAndTimeout(t *testing.T) {
	watchdog, stopWatchdog := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopWatchdog()
	limits := executionLimits()
	limits.HTTP.Timeout = 25 * time.Millisecond
	program := compileExecution(t, executionDefinition(), limits)
	client := executionClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})

	// Policy expiration returns an execution error after the unknown client handoff.
	result, err := program.Execute(watchdog, client, executionInvocation())
	require.NoError(t, err)
	requireExecutionFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
	require.NoError(t, watchdog.Err())

	// The same timeout includes projection after a complete upstream response.
	definition := executionDefinition()
	definition.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: util.ToPtr(`(() => { while (true) {} })()`)}
	limits.Response.Transform.Timeout = 10 * time.Second
	transform := compileExecution(t, definition, limits)
	body := &executionBody{reader: strings.NewReader(`true`)}
	client = executionClient(func(*http.Request) (*http.Response, error) { return executionResponse(body), nil })
	result, err = transform.Execute(watchdog, client, executionInvocation())
	require.NoError(t, err)
	requireExecutionFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
	require.NoError(t, watchdog.Err())
	require.EqualValues(t, 1, body.closes.Load())

	// Caller cancellation stays a Go error, including cancellation during a send.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client = executionClient(func(request *http.Request) (*http.Response, error) {
		cancel()
		<-request.Context().Done()
		return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private-canceled"))
	})
	result, err = program.Execute(ctx, client, executionInvocation())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	// A fresh invocation still succeeds; no previous timeout or state is retained.
	client = executionClient(func(*http.Request) (*http.Response, error) {
		return executionResponse(&executionBody{reader: strings.NewReader(`true`)}), nil
	})
	result, err = program.Execute(context.Background(), client, executionInvocation())
	require.NoError(t, err)
	require.False(t, result.IsError)
}

// TestExecuteConcurrentReuseOwnsDefinition checks that compilation captures its
// source and concurrent calls use distinct argument/context/result values.
func TestExecuteConcurrentReuseOwnsDefinition(t *testing.T) {
	definition := executionDefinition()
	definition.ProxyHTTP.Headers = map[string]string{"X-Id": "{{params.id}}"}
	definition.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: util.ToPtr(`({id: params.id, reply: response.body.id})`)}
	program := compileExecution(t, definition, executionLimits())
	// Mutating caller-owned source after compilation cannot alter execution.
	definition.ProxyHTTP.URL = "private-invalid-url"
	definition.ProxyHTTP.Headers["X-Id"] = "private-mutated"
	*definition.ProxyHTTP.Response.TransformJavascript = `false`
	copy(definition.InputSchema, strings.Repeat("!", len(definition.InputSchema)))

	var wait sync.WaitGroup
	outcomes := make(chan error, 12)
	for id := range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			invocation := executionInvocation()
			invocation.Arguments = common.RawJSON(fmt.Sprintf(`{"id":"%d"}`, id))
			client := executionClient(func(request *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"id":%q}`, request.Header.Get("X-Id"))
				return executionResponse(&executionBody{reader: strings.NewReader(body)}), nil
			})
			result, err := program.Execute(context.Background(), client, invocation)
			if err != nil {
				outcomes <- err
				return
			}
			want := fmt.Sprintf(`{"id":"%d","reply":"%d"}`, id, id)
			if result.IsError || string(result.StructuredContent) != want {
				outcomes <- fmt.Errorf("invocation %d returned %+v", id, result)
				return
			}
			outcomes <- nil
		}()
	}
	wait.Wait()
	close(outcomes)
	for err := range outcomes {
		require.NoError(t, err)
	}
}
