package httpcodec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// bodyString supplies pointer-valued body fields without sharing test state.
func bodyString(value string) *string { return &value }

// TestBodyPresence distinguishes no body from all intentionally empty bodies.
func TestBodyPresence(t *testing.T) {
	for _, test := range []struct {
		name, contentType, payload string
		source                     tools.ProxyHTTP
		present                    bool
	}{
		{name: "absent"},
		{name: "JSON null", source: tools.ProxyHTTP{BodyJSON: common.RawJSON(`null`)}, present: true, contentType: "application/json", payload: "null"},
		{name: "empty text", source: tools.ProxyHTTP{BodyTemplate: &tools.BodyTemplate{MediaType: "text/plain"}}, present: true, contentType: "text/plain"},
		{name: "empty raw", source: tools.ProxyHTTP{BodyRaw: bodyString("")}, present: true},
		{name: "empty form", source: tools.ProxyHTTP{Form: &tools.FormBody{Fields: map[string]common.RawJSON{}}}, present: true, contentType: "application/x-www-form-urlencoded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Body compilation intentionally does not require a method or URL.
			program, err := httpcodec.CompileBody(&test.source, parameterLimits())
			require.NoError(t, err)
			result, err := program.Render(context.Background(), parameterContext(t, render.Input{}))

			// Present preserves the caller's body intent even when there are no bytes.
			require.NoError(t, err)
			require.Equal(t, test.present, result.Present)
			require.Equal(t, test.contentType, result.ContentType)
			require.Equal(t, test.payload, string(result.Bytes))
		})
	}
}

// TestBodyJSONPreservesTypes keeps typed values and exact numeric spellings.
func TestBodyJSONPreservesTypes(t *testing.T) {
	// A copied invocation supplies scalar, repeated, null, and inert template data.
	data := parameterContext(t, render.Input{Params: map[string]any{
		"enabled": false, "items": []any{nil, false, "{{cfg.hidden}}"},
		"number": json.Number("9007199254740993.0100"),
	}})
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{BodyJSON: common.RawJSON(`{
		"enabled":{"$value":"params.enabled"},"items":{"$value":"params.items"},
		"number":{"$value":"params.number"},"literal":{"$literal":"{{>local-file}}"}
	}`)}, parameterLimits())
	require.NoError(t, err)

	// Typed insertion never reevaluates strings from invocation data or literals.
	result, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "application/json", result.ContentType)
	require.Equal(t, `{"enabled":false,"items":[null,false,"{{cfg.hidden}}"],"literal":"{{>local-file}}","number":9007199254740993.0100}`, string(result.Bytes))
}

// TestBodyTemplateJSONMediaTypes validates final JSON according to the concrete
// media subtype while leaving other text and quoted MIME parameters unchanged.
func TestBodyTemplateJSONMediaTypes(t *testing.T) {
	for _, mediaType := range []string{"application/json", "APPLICATION/JSON; Charset=UTF-8", "application/problem+json", `Application/Problem+JSON; profile="{{cfg.absent}}"`} {
		t.Run(mediaType, func(t *testing.T) {
			// Only the payload is executable template source; MIME parameters stay literal.
			program, err := httpcodec.CompileBody(&tools.ProxyHTTP{BodyTemplate: &tools.BodyTemplate{
				MediaType: mediaType, Template: "{{params.payload}}",
			}}, parameterLimits())
			require.NoError(t, err)
			result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"payload": ` [false,null] `}}))
			require.NoError(t, err)
			require.Equal(t, mediaType, result.ContentType)
			require.Equal(t, ` [false,null] `, string(result.Bytes))

			// Invalid or trailing JSON fails without returning resolved private content.
			for _, payload := range []string{"private-configuration", `null true`, ""} {
				result, err = program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"payload": payload}}))
				require.Error(t, err)
				require.Equal(t, httpcodec.RenderedBody{}, result)
				require.NotContains(t, err.Error(), "private-configuration")
			}
		})
	}

	// A non-JSON body keeps ordinary text byte-for-byte, including line breaks.
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{BodyTemplate: &tools.BodyTemplate{
		MediaType: "text/plain", Template: "{{params.payload}}",
	}}, parameterLimits())
	require.NoError(t, err)
	result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"payload": "not JSON\nsecond line"}}))
	require.NoError(t, err)
	require.Equal(t, "not JSON\nsecond line", string(result.Bytes))
}

// TestBodyRawDecodesRenderedData validates resolved base64 before exposing bytes.
func TestBodyRawDecodesRenderedData(t *testing.T) {
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{BodyRaw: bodyString("{{params.payload}}")}, parameterLimits())
	require.NoError(t, err)

	// Binary output is preserved without imposing a content type or UTF-8 encoding.
	result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"payload": "AP+A"}}))
	require.NoError(t, err)
	require.Equal(t, []byte{0, 255, 128}, result.Bytes)
	require.Empty(t, result.ContentType)

	// A malformed resolved value returns neither partial data nor the private value.
	result, err = program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{"payload": "private-configuration!"}}))
	require.Error(t, err)
	require.Equal(t, httpcodec.RenderedBody{}, result)
	require.NotContains(t, err.Error(), "private-configuration")
}

// TestBodyFormReusesParameterRules retains optional omissions, repeated scalars,
// precise numbers, and ordinary application/x-www-form-urlencoded escaping.
func TestBodyFormReusesParameterRules(t *testing.T) {
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{Form: &tools.FormBody{Fields: map[string]common.RawJSON{
		"cursor": common.RawJSON(`{"$optional":"params.cursor"}`),
		"values": common.RawJSON(`{"$value":"params.values"}`),
		"text":   common.RawJSON(`"{{params.text}}"`),
	}}}, parameterLimits())
	require.NoError(t, err)

	// Missing optional data disappears while false, null, and repeated values survive.
	result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{
		"values": []any{false, nil, json.Number("9007199254740993")}, "text": "a&b +é",
	}}))
	require.NoError(t, err)
	require.Equal(t, "application/x-www-form-urlencoded", result.ContentType)
	require.Equal(t, "text=a%26b+%2B%C3%A9&values=false&values=null&values=9007199254740993", string(result.Bytes))
}

// TestBodyMultipartRendersMetadataAndPayload preserves part order and literal
// MIME types while templating names, filenames, and both payload modes.
func TestBodyMultipartRendersMetadataAndPayload(t *testing.T) {
	// Both parts deliberately use the same rendered name to model repeated uploads.
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{
		{Name: "{{params.name}}", Text: bodyString("{{params.text}}"), MediaType: `text/plain; note="{{cfg.absent}}"`},
		{Name: "{{params.name}}", Filename: "{{params.filename}}", BodyRaw: bodyString("{{params.binary}}")},
	}}}, parameterLimits())
	require.NoError(t, err)
	result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: map[string]any{
		"name": "uploads", "text": "{{cfg.absent}}", "filename": "report-é.bin", "binary": "AP+A",
	}}))
	require.NoError(t, err)
	require.True(t, result.Present)

	// A standard MIME reader observes the final boundary and the authored part order.
	mediaType, params, err := mime.ParseMediaType(result.ContentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(result.Bytes), params["boundary"])
	for _, expected := range []struct {
		filename, contentType string
		payload               []byte
	}{
		{"", `text/plain; note="{{cfg.absent}}"`, []byte("{{cfg.absent}}")},
		{"report-é.bin", "application/octet-stream", []byte{0, 255, 128}},
	} {
		part, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, "uploads", part.FormName())
		require.Equal(t, expected.filename, part.FileName())
		require.Equal(t, expected.contentType, part.Header.Get("Content-Type"))
		payload, err := io.ReadAll(part)
		require.NoError(t, err)
		require.Equal(t, expected.payload, payload)
	}
	_, err = reader.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

// TestBodyMultipartResolvedValidation rejects invocation data that would create
// invalid MIME metadata or base64, without returning partial MIME output.
func TestBodyMultipartResolvedValidation(t *testing.T) {
	for _, field := range []string{"name", "filename", "binary"} {
		t.Run(field, func(t *testing.T) {
			program, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{{
				Name: "{{params.name}}", Filename: "{{params.filename}}", BodyRaw: bodyString("{{params.binary}}"),
			}}}}, parameterLimits())
			require.NoError(t, err)

			// One dynamic field is invalid while all other rendered fields are valid.
			params := map[string]any{"name": "file", "filename": "report.bin", "binary": "Zg=="}
			params[field] = "private-configuration\x00!"
			result, err := program.Render(context.Background(), parameterContext(t, render.Input{Params: params}))
			require.Error(t, err)
			require.Equal(t, httpcodec.RenderedBody{}, result)
			require.NotContains(t, err.Error(), "private-configuration")
		})
	}
}

// TestBodyOwnsSourceAndResults checks compiled programs retain no caller-owned
// source buffers or pointers, and returned payloads may be modified independently.
func TestBodyOwnsSourceAndResults(t *testing.T) {
	for _, mode := range []string{"JSON", "text", "binary", "form", "multipart"} {
		t.Run(mode, func(t *testing.T) {
			// Keep references to every source container after compilation.
			text := "original"
			source := tools.ProxyHTTP{}
			switch mode {
			case "JSON":
				source.BodyJSON = common.RawJSON(`"original"`)
			case "text":
				source.BodyTemplate = &tools.BodyTemplate{MediaType: "text/plain", Template: text}
			case "binary":
				text = "b3JpZ2luYWw="
				source.BodyRaw = &text
			case "form":
				source.Form = &tools.FormBody{Fields: map[string]common.RawJSON{"value": common.RawJSON(`"original"`)}}
			case "multipart":
				source.Multipart = &tools.MultipartBody{Parts: []tools.MultipartPart{{Name: "value", Text: &text, MediaType: "text/plain"}}}
			}
			program, err := httpcodec.CompileBody(&source, parameterLimits())
			require.NoError(t, err)

			// Replace source metadata and corrupt referenced payload bytes and pointers.
			text = "changed"
			if source.BodyJSON != nil {
				source.BodyJSON[1] = '!'
			}
			if source.BodyTemplate != nil {
				*source.BodyTemplate = tools.BodyTemplate{MediaType: "application/json", Template: "changed"}
			}
			if source.Form != nil {
				source.Form.Fields["value"][1] = '!'
				delete(source.Form.Fields, "value")
			}
			if source.Multipart != nil {
				source.Multipart.Parts[0].Name = "changed"
				source.Multipart.Parts[0].MediaType = "application/json"
			}
			data := parameterContext(t, render.Input{})
			first, err := program.Render(context.Background(), data)
			require.NoError(t, err)
			require.Contains(t, string(first.Bytes), "original")
			require.NotContains(t, string(first.Bytes), "changed")
			require.NotContains(t, string(first.Bytes), "application/json")

			// Destroy a returned payload; a later render must allocate independent bytes.
			for i := range first.Bytes {
				first.Bytes[i] = '!'
			}
			second, err := program.Render(context.Background(), data)
			require.NoError(t, err)
			require.Contains(t, string(second.Bytes), "original")
		})
	}
}

// TestBodyConcurrentReuse gives each invocation independent MIME state and data
// even when many callers share one compiled multipart definition.
func TestBodyConcurrentReuse(t *testing.T) {
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{{
		Name: "value", Text: bodyString("{{params.value}}"),
	}}}}, parameterLimits())
	require.NoError(t, err)
	const workers = 24
	failures := make(chan error, workers)
	var group sync.WaitGroup

	// Each goroutine renders and reads only its own invocation's final MIME payload.
	for i := 0; i < workers; i++ {
		value := fmt.Sprintf("worker-%d", i)
		data := parameterContext(t, render.Input{Params: map[string]any{"value": value}})
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := program.Render(context.Background(), data)
			if err == nil {
				_, params, parseErr := mime.ParseMediaType(result.ContentType)
				err = parseErr
				if err == nil {
					part, partErr := multipart.NewReader(bytes.NewReader(result.Bytes), params["boundary"]).NextPart()
					err = partErr
					if err == nil {
						payload, readErr := io.ReadAll(part)
						err = readErr
						if err == nil && string(payload) != value {
							err = fmt.Errorf("expected %q, received %q", value, payload)
						}
					}
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

// TestBodyCompilationRejectsInvalidDefinitions preserves explicit-null errors
// and body exclusivity before constructing a reusable program.
func TestBodyCompilationRejectsInvalidDefinitions(t *testing.T) {
	for _, document := range []string{
		`{"bodyTemplate":null}`, `{"bodyRaw":null}`, `{"form":null}`, `{"multipart":null}`,
		`{"bodyJson":null,"bodyRaw":""}`, `{"bodyTemplate":{"mediaType":"text/plain","template":""},"form":{"fields":{}}}`,
		`{"bodyTemplate":{"mediaType":"text/plain","template":"{{>local-file}}"}}`,
		`{"bodyTemplate":{"mediaType":"application","template":"text"}}`,
		`{"multipart":{"parts":[{"name":"value","text":"text","mediaType":"text/*"}]}}`,
	} {
		t.Run(document, func(t *testing.T) {
			// Decode into the real schema to retain private presence markers.
			var source tools.ProxyHTTP
			require.NoError(t, json.Unmarshal([]byte(document), &source))
			program, err := httpcodec.CompileBody(&source, parameterLimits())
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// TestBodyCompilationLimits rejects disabled caps and oversized sources before
// parsing or decoding bodies, including aggregate multipart source data.
func TestBodyCompilationLimits(t *testing.T) {
	for _, limits := range []render.Limits{
		{MaxDepth: 8, MaxSteps: 100}, {MaxBytes: 100, MaxSteps: 100}, {MaxBytes: 100, MaxDepth: 8},
	} {
		program, err := httpcodec.CompileBody(&tools.ProxyHTTP{}, limits)
		require.Error(t, err)
		require.Nil(t, program)
	}

	// Even invalid syntax must hit the source byte cap before downstream parsing.
	for _, source := range []tools.ProxyHTTP{
		{BodyJSON: common.RawJSON(strings.Repeat("{", 65))},
		{BodyTemplate: &tools.BodyTemplate{MediaType: "text/plain", Template: strings.Repeat("x", 55)}},
		{BodyRaw: bodyString(strings.Repeat("!", 65))},
		{Form: &tools.FormBody{Fields: map[string]common.RawJSON{strings.Repeat("k", 64): common.RawJSON(`0`)}}},
		{Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{{Name: "first", Text: bodyString(strings.Repeat("x", 30))}, {Name: "second", Text: bodyString(strings.Repeat("x", 30))}}}},
	} {
		limits := parameterLimits()
		limits.MaxBytes = 64
		program, err := httpcodec.CompileBody(&source, limits)
		require.ErrorContains(t, err, "source byte limit")
		require.Nil(t, program)
	}
}

// TestBodyMultipartSharedLimits charges the combined rendered value and syntax
// work once, then separately caps the complete MIME message including framing.
func TestBodyMultipartSharedLimits(t *testing.T) {
	for _, kind := range []string{"rendered bytes", "render steps"} {
		t.Run(kind, func(t *testing.T) {
			limits := parameterLimits()
			template := "{{params.value}}"
			params := map[string]any{"value": strings.Repeat("x", 400)}
			limits.MaxBytes = 700
			if kind == "render steps" {
				limits = parameterLimits()
				limits.MaxSteps = 100
				template = "{{#params.values}}{{.}}{{/params.values}}"
				values := make([]any, 30)
				for i := range values {
					values[i] = i
				}
				params = map[string]any{"values": values}
			}
			data := parameterContext(t, render.Input{Params: params})
			parts := []tools.MultipartPart{{Name: "a", Text: &template}, {Name: "b", Text: &template}}

			// Each part is individually affordable with the same invocation and limits.
			one, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: parts[:1]}}, limits)
			require.NoError(t, err)
			_, err = one.Render(context.Background(), data)
			require.NoError(t, err)

			// Their combined rendering must not receive separate per-part allowances.
			both, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: parts}}, limits)
			require.NoError(t, err)
			result, err := both.Render(context.Background(), data)
			require.Error(t, err)
			require.Contains(t, err.Error(), "render")
			require.Equal(t, httpcodec.RenderedBody{}, result)
		})
	}

	// A tiny payload and its compiled source fit, but its MIME headers do not.
	limits := parameterLimits()
	limits.MaxBytes = 128
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{Multipart: &tools.MultipartBody{Parts: []tools.MultipartPart{{Name: "a", Text: bodyString("x")}}}}, limits)
	require.NoError(t, err)
	result, err := program.Render(context.Background(), parameterContext(t, render.Input{}))
	require.ErrorContains(t, err, "encoded multipart byte limit")
	require.Equal(t, httpcodec.RenderedBody{}, result)
}

// TestBodyRequiredArgumentsAndCancellation rejects incomplete calls and honors
// cancellation even when an absent body would otherwise require no rendering.
func TestBodyRequiredArgumentsAndCancellation(t *testing.T) {
	program, err := httpcodec.CompileBody(&tools.ProxyHTTP{}, parameterLimits())
	require.NoError(t, err)
	data := parameterContext(t, render.Input{})
	var nilProgram *httpcodec.Body
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// No invalid or canceled invocation may expose a present or partial body.
	for _, test := range []struct {
		name    string
		program *httpcodec.Body
		ctx     context.Context
		data    *render.Context
	}{
		{"nil body", nilProgram, context.Background(), data},
		{"zero body", &httpcodec.Body{}, context.Background(), data},
		{"nil context", program, nil, data},
		{"nil data", program, context.Background(), nil},
		{"canceled", program, canceled, data},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.program.Render(test.ctx, test.data)
			require.Error(t, err)
			require.Equal(t, httpcodec.RenderedBody{}, result)
			if test.name == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
	program, err = httpcodec.CompileBody(nil, parameterLimits())
	require.Error(t, err)
	require.Nil(t, program)
}
