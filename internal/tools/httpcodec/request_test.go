package httpcodec_test

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// readRequestBody consumes and closes a successful prepared request's body.
func readRequestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	require.NotNil(t, request.Body)
	payload, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.NoError(t, request.Body.Close())
	return payload
}

// TestRequestRender assembles connection configuration, escaped path input,
// repeated query values, literal header names, and a typed JSON body together.
func TestRequestRender(t *testing.T) {
	// Outbound compilation preserves methods and leaves response policy to execution.
	source := tools.ProxyHTTP{
		Method: "patch-Custom", URL: "https://{{cfg.host}}/tenants/{{path.cfg.tenant}}/contacts/{{path.params.id}}?keep=a%20b&flag",
		Headers: map[string]string{
			"Authorization": "Bearer author-defined", "hOsT": "{{cfg.override}}",
			"$value": "{{params.header}}", "X-Tenant": "{{cfg.tenant}}",
		},
		Query:    map[string]common.RawJSON{"tag": common.RawJSON(`{"$value":"params.tags"}`)},
		BodyJSON: common.RawJSON(`{"enabled":{"$value":"params.enabled"},"name":"{{params.name}}"}`),
		Response: &tools.HTTPResponse{TransformJavascript: bodyString("")},
	}
	program, err := httpcodec.CompileRequest(&source, parameterLimits())
	require.NoError(t, err)
	data := parameterContext(t, render.Input{
		Cfg:    map[string]any{"host": "api.example.test", "tenant": "acme/team", "override": "tenant.example.test:8443"},
		Params: map[string]any{"id": "sales/42", "tags": []any{"lead & web", "new"}, "enabled": false, "name": "Ada", "header": "literal-name"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := program.Render(ctx, data)

	// Host routing stays distinct from the destination URL, and authentication is untouched.
	require.NoError(t, err)
	require.Equal(t, source.Method, request.Method)
	require.Equal(t, "https://api.example.test/tenants/acme%2Fteam/contacts/sales%2F42?keep=a%20b&flag&tag=lead+%26+web&tag=new", request.URL.String())
	require.Equal(t, "tenant.example.test:8443", request.Host)
	require.Empty(t, request.Header.Get("Host"))
	require.Equal(t, "Bearer author-defined", request.Header.Get("Authorization"))
	require.Equal(t, "literal-name", request.Header.Get("$value"))
	require.Equal(t, "acme/team", request.Header.Get("X-Tenant"))
	require.Equal(t, "application/json", request.Header.Get("Content-Type"))
	require.Same(t, ctx, request.Context())
	require.Equal(t, `{"enabled":false,"name":"Ada"}`, string(readRequestBody(t, request)))
}

// TestRequestQueryCollisionOnlyForEmittedValues preserves raw query spelling
// and lets an omitted structured field coexist with a URL-owned parameter.
func TestRequestQueryCollisionOnlyForEmittedValues(t *testing.T) {
	program, err := httpcodec.CompileRequest(&tools.ProxyHTTP{
		Method: "GET", URL: "https://example.test/?%63ursor=first%20page&tag=raw&tag=again",
		Query: map[string]common.RawJSON{"cursor": common.RawJSON(`{"$optional":"params.cursor"}`)},
	}, parameterLimits())
	require.NoError(t, err)
	for _, value := range []any{nil, []any{}} {
		// Optional null and empty repeated values emit no colliding parameter.
		request, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"cursor": value}}))
		require.NoError(t, err)
		require.Equal(t, "%63ursor=first%20page&tag=raw&tag=again", request.URL.RawQuery)
		require.Nil(t, request.Body)
	}

	// A present value collides with the decoded key, even when its URL spelling is escaped.
	request, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"cursor": "private-cursor"}}))
	require.Error(t, err)
	require.Nil(t, request)
	require.NotContains(t, err.Error(), "private-cursor")
}

// TestRequestBodyAndContentType carries generated MIME types into the final
// request while retaining compatible authored types and additional parameters.
func TestRequestBodyAndContentType(t *testing.T) {
	for _, test := range []struct {
		name, declared, base, payload string
		params                        map[string]string
		body                          tools.ProxyHTTP
	}{
		{name: "JSON default", base: "application/json", payload: "null", body: tools.ProxyHTTP{BodyJSON: common.RawJSON(`null`)}},
		{name: "JSON vendor type", declared: "application/vnd.acme+json; profile=contact", base: "application/vnd.acme+json", params: map[string]string{"profile": "contact"}, payload: "false", body: tools.ProxyHTTP{BodyJSON: common.RawJSON(`false`)}},
		{name: "template parameters", declared: "TEXT/PLAIN; charset=utf-8; format=flowed", base: "text/plain", params: map[string]string{"charset": "utf-8", "format": "flowed"}, payload: "hello", body: tools.ProxyHTTP{BodyTemplate: &tools.BodyTemplate{MediaType: "text/plain; charset=utf-8", Template: "hello"}}},
		{name: "form default", base: "application/x-www-form-urlencoded", payload: "name=Ada+Lovelace", body: tools.ProxyHTTP{Form: &tools.FormBody{Fields: map[string]common.RawJSON{"name": common.RawJSON(`"Ada Lovelace"`)}}}},
		{name: "form extra charset", declared: "application/x-www-form-urlencoded; charset=utf-8", base: "application/x-www-form-urlencoded", params: map[string]string{"charset": "utf-8"}, payload: "", body: tools.ProxyHTTP{Form: &tools.FormBody{Fields: map[string]common.RawJSON{}}}},
		{name: "raw without type", payload: "\x00\xff\x80", body: tools.ProxyHTTP{BodyRaw: bodyString("AP+A")}},
		{name: "raw authored type", declared: "application/octet-stream", base: "application/octet-stream", payload: "data", body: tools.ProxyHTTP{BodyRaw: bodyString("ZGF0YQ==")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The body mode chooses defaults; compatible authored metadata may refine them.
			source := test.body
			source.Method, source.URL = "POST", "https://example.test/upload"
			if test.declared != "" {
				source.Headers = map[string]string{"Content-Type": test.declared}
			}
			program, err := httpcodec.CompileRequest(&source, parameterLimits())
			require.NoError(t, err)
			request, err := program.Render(context.Background(), parameterContext(t, render.Input{}))
			require.NoError(t, err)
			require.Equal(t, test.payload, string(readRequestBody(t, request)))
			require.Equal(t, int64(len(test.payload)), request.ContentLength)
			if test.base == "" {
				require.Empty(t, request.Header.Get("Content-Type"))
			} else {
				base, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
				require.NoError(t, err)
				require.Equal(t, test.base, base)
				if test.params == nil {
					require.Empty(t, params)
				} else {
					require.Equal(t, test.params, params)
				}
			}
		})
	}
}

// TestRequestMultipartBoundary pairs the generated boundary with its bytes,
// retains declared parameters, and prevents authors from selecting a boundary.
func TestRequestMultipartBoundary(t *testing.T) {
	source := tools.ProxyHTTP{
		Method: "POST", URL: "https://example.test/upload",
		Headers:   map[string]string{"Content-Type": "multipart/form-data; profile=upload"},
		Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{{Name: "field", Text: bodyString("value")}}},
	}
	program, err := httpcodec.CompileRequest(&source, parameterLimits())
	require.NoError(t, err)
	request, err := program.Render(context.Background(), parameterContext(t, render.Input{}))
	require.NoError(t, err)

	// The standard MIME reader must parse the actual bytes using the emitted header.
	base, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", base)
	require.Equal(t, "upload", params["profile"])
	require.NotEmpty(t, params["boundary"])
	reader := multipart.NewReader(request.Body, params["boundary"])
	part, err := reader.NextPart()
	require.NoError(t, err)
	payload, err := io.ReadAll(part)
	require.NoError(t, err)
	require.Equal(t, "field", part.FormName())
	require.Equal(t, "value", string(payload))
	_, err = reader.NextPart()
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, request.Body.Close())

	// A boundary introduced by an evaluated header cannot replace generated framing.
	source.Headers["Content-Type"] = "{{params.mediaType}}"
	program, err = httpcodec.CompileRequest(&source, parameterLimits())
	require.NoError(t, err)
	request, err = program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{
		"mediaType": "multipart/form-data; boundary=private-boundary",
	}}))
	require.Error(t, err)
	require.Nil(t, request)
	require.NotContains(t, err.Error(), "private-boundary")
}

// TestRequestAbsentAndEmptyBodies preserves omission separately from a present
// empty body and supplies replay readers for every intentionally present body.
func TestRequestAbsentAndEmptyBodies(t *testing.T) {
	for _, mode := range []string{"absent", "text", "raw", "form"} {
		t.Run(mode, func(t *testing.T) {
			source := tools.ProxyHTTP{Method: "POST", URL: "https://example.test"}
			switch mode {
			case "text":
				source.BodyTemplate = &tools.BodyTemplate{MediaType: "text/plain"}
			case "raw":
				source.BodyRaw = bodyString("")
			case "form":
				source.Form = &tools.FormBody{Fields: map[string]common.RawJSON{}}
			}
			program, err := httpcodec.CompileRequest(&source, parameterLimits())
			require.NoError(t, err)
			request, err := program.Render(context.Background(), parameterContext(t, render.Input{}))
			require.NoError(t, err)

			// HTTP's explicit NoBody sentinel retains the authored empty-body intent.
			if mode == "absent" {
				require.Nil(t, request.Body)
				require.Nil(t, request.GetBody)
			} else {
				require.Equal(t, http.NoBody, request.Body)
				require.NotNil(t, request.GetBody)
				replay, err := request.GetBody()
				require.NoError(t, err)
				require.Equal(t, http.NoBody, replay)
				require.NoError(t, replay.Close())
			}
		})
	}
}

// TestRequestOwnsSourceAndReplay keeps compiled source, invocation outputs, and
// replay readers independent when callers mutate their retained objects.
func TestRequestOwnsSourceAndReplay(t *testing.T) {
	source := tools.ProxyHTTP{
		Method: "POST", URL: "https://example.test/original", Headers: map[string]string{"X-Name": "original"},
		Query: map[string]common.RawJSON{"q": common.RawJSON(`"original"`)}, BodyJSON: common.RawJSON(`"original"`),
	}
	program, err := httpcodec.CompileRequest(&source, parameterLimits())
	require.NoError(t, err)

	// Change every caller-owned source container after the program has captured it.
	source.Method, source.URL = "DELETE", "https://changed.test"
	source.Headers["X-Name"] = "changed"
	source.Query["q"][1] = '!'
	source.BodyJSON[1] = '!'
	data := parameterContext(t, render.Input{})
	first, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, `"original"`, string(readRequestBody(t, first)))

	// Closing the original stream does not consume independent retry/recovery readers.
	for i := 0; i < 2; i++ {
		replay, err := first.GetBody()
		require.NoError(t, err)
		payload, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.Equal(t, `"original"`, string(payload))
		require.NoError(t, replay.Close())
	}
	first.Header.Set("X-Name", "changed")
	first.URL.Host, first.URL.Path, first.Host = "changed.test", "/changed", "changed.test"

	// Each invocation receives fresh URL, headers, and body state.
	second, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "POST", second.Method)
	require.Equal(t, "https://example.test/original?q=original", second.URL.String())
	require.Equal(t, "example.test", second.Host)
	require.Equal(t, "original", second.Header.Get("X-Name"))
	require.Equal(t, `"original"`, string(readRequestBody(t, second)))
}

// TestRequestConcurrentReuse renders independent headers, URLs, and streams for
// many invocations sharing the same compiled outbound request definition.
func TestRequestConcurrentReuse(t *testing.T) {
	program, err := httpcodec.CompileRequest(&tools.ProxyHTTP{
		Method: "POST", URL: "https://example.test/{{params.id}}", Headers: map[string]string{"X-ID": "{{params.id}}"},
		Query:    map[string]common.RawJSON{"id": common.RawJSON(`{"$value":"params.id"}`)},
		BodyJSON: common.RawJSON(`{"$value":"params.id"}`),
	}, parameterLimits())
	require.NoError(t, err)
	const workers = 24
	failures := make(chan error, workers)
	var group sync.WaitGroup
	for id := 0; id < workers; id++ {
		// Each worker captures distinct input before sharing only the compiled program.
		data := parameterContext(t, render.Input{Params: map[string]any{"id": id}})
		group.Add(1)
		go func() {
			defer group.Done()
			request, err := program.Render(context.Background(), data)
			if err == nil {
				payload, readErr := io.ReadAll(request.Body)
				err = readErr
				closeErr := request.Body.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil && (request.URL.String() != fmt.Sprintf("https://example.test/%d?id=%d", id, id) || request.Header.Get("X-ID") != fmt.Sprint(id) || string(payload) != fmt.Sprint(id)) {
					err = fmt.Errorf("worker %d received another invocation's request data", id)
				}
			}
			failures <- err
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
}

// TestRequestRejectsResolvedValues validates URL and header output before any
// request escapes, without revealing configuration values in error messages.
func TestRequestRejectsResolvedValues(t *testing.T) {
	for _, field := range []string{"url", "header", "host", "content-type"} {
		t.Run(field, func(t *testing.T) {
			source := tools.ProxyHTTP{Method: "POST", URL: "https://example.test", BodyJSON: common.RawJSON(`null`)}
			value := "private-configuration\x00"
			switch field {
			case "url":
				source.URL = "{{cfg.value}}"
				value = "https://private-configuration.test/#fragment"
			case "header":
				source.Headers = map[string]string{"X-Test": "{{cfg.value}}"}
			case "host":
				source.Headers = map[string]string{"Host": "{{cfg.value}}"}
			case "content-type":
				source.Headers = map[string]string{"Content-Type": "{{cfg.value}}"}
			}
			program, err := httpcodec.CompileRequest(&source, parameterLimits())
			require.NoError(t, err)

			// Dynamic validation fails atomically even though the body itself is valid.
			request, err := program.Render(context.Background(), parameterContext(t, render.Input{Cfg: map[string]any{"value": value}}))
			require.Error(t, err)
			require.Nil(t, request)
			require.NotContains(t, err.Error(), "private-configuration")
		})
	}

	// A valid MIME type can still conflict with the selected body mode or parameters.
	for _, body := range []tools.ProxyHTTP{
		{BodyJSON: common.RawJSON(`null`)},
		{BodyTemplate: &tools.BodyTemplate{MediaType: "text/plain; charset=utf-8", Template: "text"}},
		{Form: &tools.FormBody{Fields: map[string]common.RawJSON{}}},
	} {
		body.Method, body.URL = "POST", "https://example.test"
		body.Headers = map[string]string{"Content-Type": "{{params.mediaType}}"}
		program, err := httpcodec.CompileRequest(&body, parameterLimits())
		require.NoError(t, err)
		request, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"mediaType": "text/plain; charset=latin-1"}}))
		require.Error(t, err)
		require.Nil(t, request)
	}
}

// TestRequestCompileRejectsStaticErrors fails known-invalid outbound metadata
// before retaining a reusable program or consulting invocation context.
func TestRequestCompileRejectsStaticErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*tools.ProxyHTTP)
	}{
		{"method", func(p *tools.ProxyHTTP) { p.Method = "BAD METHOD" }},
		{"relative URL", func(p *tools.ProxyHTTP) { p.URL = "/relative" }},
		{"fragment", func(p *tools.ProxyHTTP) { p.URL += "#fragment" }},
		{"duplicate headers", func(p *tools.ProxyHTTP) { p.Headers = map[string]string{"X-Name": "a", "x-name": "b"} }},
		{"hop header", func(p *tools.ProxyHTTP) { p.Headers = map[string]string{"Connection": "keep-alive"} }},
		{"framing header", func(p *tools.ProxyHTTP) { p.Headers = map[string]string{"Content-Length": "4"} }},
		{"competing body", func(p *tools.ProxyHTTP) { p.BodyJSON, p.BodyRaw = common.RawJSON(`null`), bodyString("") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := tools.ProxyHTTP{Method: "GET", URL: "https://example.test"}
			test.change(&source)
			program, err := httpcodec.CompileRequest(&source, parameterLimits())
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// TestRequestSharedEnvelopeLimits ensures URL/header fields share work and
// rendered-byte allowances instead of receiving one allowance per field.
func TestRequestSharedEnvelopeLimits(t *testing.T) {
	for _, kind := range []string{"bytes", "steps"} {
		t.Run(kind, func(t *testing.T) {
			limits := parameterLimits()
			template := "{{params.value}}"
			input := render.Input{Params: map[string]any{"value": strings.Repeat("x", 180)}}
			limits.MaxBytes = 320
			if kind == "steps" {
				limits = parameterLimits()
				limits.MaxSteps = 100
				template = "{{#params.items}}{{.}}{{/params.items}}"
				items := make([]any, 30)
				for i := range items {
					items[i] = i
				}
				input.Params = map[string]any{"items": items}
			}
			data := parameterContext(t, input)

			// Either expanded field is affordable by itself using these same limits.
			for _, field := range []string{"url", "header", "both"} {
				source := tools.ProxyHTTP{Method: "GET", URL: "https://example.test/"}
				if field != "header" {
					source.URL += template
				}
				if field != "url" {
					source.Headers = map[string]string{"X-Value": template}
				}
				program, err := httpcodec.CompileRequest(&source, limits)
				require.NoError(t, err)
				request, err := program.Render(context.Background(), data)
				if field == "both" {
					// The envelope fails first, before final prepared-size validation.
					require.ErrorContains(t, err, "render")
					require.Nil(t, request)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

// TestRequestSourceAndPreparedByteLimits checks the aggregate boundaries across
// stages, including effective Host and generated content-type header bytes.
func TestRequestSourceAndPreparedByteLimits(t *testing.T) {
	// Each field fits by itself, but source data across envelope/query/body does not.
	limits := parameterLimits()
	limits.MaxBytes = 128
	program, err := httpcodec.CompileRequest(&tools.ProxyHTTP{
		Method: "POST", URL: "https://example.test", Headers: map[string]string{"X-Info": strings.Repeat("x", 40)},
		Query:   map[string]common.RawJSON{"q": common.RawJSON(`"` + strings.Repeat("x", 40) + `"`)},
		BodyRaw: bodyString(strings.Repeat("Zg==", 20)),
	}, limits)
	require.ErrorContains(t, err, "source byte limit")
	require.Nil(t, program)

	// Every stage's rendered data fits; only their complete prepared request reaches the cap.
	source := tools.ProxyHTTP{
		Method: "POST", URL: "https://example.test/{{params.value}}", Headers: map[string]string{"X-Info": "{{params.value}}", "Host": "{{cfg.host}}"},
		Query:        map[string]common.RawJSON{"q": common.RawJSON(`{"$value":"params.value"}`)},
		BodyTemplate: &tools.BodyTemplate{MediaType: "text/plain", Template: "{{params.value}}"},
	}
	value, host := strings.Repeat("x", 80), "upstream.example"
	data := parameterContext(t, render.Input{Params: map[string]any{"value": value}, Cfg: map[string]any{"host": host}})
	preparedURL := "https://example.test/" + value + "?q=" + value
	limits = parameterLimits()
	limits.MaxBytes = len("POST") + len(preparedURL) + len(host) + len("X-Info") + len(value) + len("Content-Type") + len("text/plain") + len(value)
	program, err = httpcodec.CompileRequest(&source, limits)
	require.NoError(t, err)
	request, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, preparedURL, request.URL.String())
	require.Equal(t, value, string(readRequestBody(t, request)))

	// Reducing only the aggregate allowance by one returns no partial request.
	limits.MaxBytes--
	program, err = httpcodec.CompileRequest(&source, limits)
	require.NoError(t, err)
	request, err = program.Render(context.Background(), data)
	require.ErrorContains(t, err, "prepared request byte limit")
	require.Nil(t, request)
}

// TestRequestRequiredArgumentsAndCancellation rejects invalid programs and
// disabled limits, and honors cancellation before returning outbound effects.
func TestRequestRequiredArgumentsAndCancellation(t *testing.T) {
	for _, limits := range []render.Limits{
		{MaxDepth: 8, MaxSteps: 100}, {MaxBytes: 100, MaxSteps: 100}, {MaxBytes: 100, MaxDepth: 8},
	} {
		program, err := httpcodec.CompileRequest(&tools.ProxyHTTP{Method: "GET", URL: "https://example.test"}, limits)
		require.Error(t, err)
		require.Nil(t, program)
	}
	program, err := httpcodec.CompileRequest(nil, parameterLimits())
	require.Error(t, err)
	require.Nil(t, program)
	program, err = httpcodec.CompileRequest(&tools.ProxyHTTP{Method: "GET", URL: "https://example.test"}, parameterLimits())
	require.NoError(t, err)
	data := parameterContext(t, render.Input{})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		program *httpcodec.Request
		ctx     context.Context
		data    *render.Context
	}{
		{"nil program", nil, context.Background(), data},
		{"zero program", &httpcodec.Request{}, context.Background(), data},
		{"nil context", program, nil, data},
		{"nil data", program, context.Background(), nil},
		{"canceled", program, canceled, data},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Even absent-body requests must have valid inputs and an active invocation.
			request, err := test.program.Render(test.ctx, test.data)
			require.Error(t, err)
			require.Nil(t, request)
			if test.name == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}
