package httpcodec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// parameterLimits permits ordinary fixtures while keeping all limits explicit.
func parameterLimits() render.Limits {
	return render.Limits{MaxBytes: 1 << 20, MaxDepth: 32, MaxSteps: 10000}
}

// parameterContext captures invocation data independently of the codec limits.
func parameterContext(t *testing.T, input render.Input) *render.Context {
	t.Helper()
	data, err := render.NewContext(input, parameterLimits())
	require.NoError(t, err)
	return data
}

// TestParametersRender combines authored templates and typed references without
// losing scalar types, numeric precision, or repeated-value order.
func TestParametersRender(t *testing.T) {
	// A connection value and caller inputs exercise raw text and typed insertion.
	data := parameterContext(t, render.Input{
		Cfg: map[string]any{"tenant": "acme"},
		Params: map[string]any{
			"search": "name email+tag&é", "enabled": false,
			"number": json.Number("9007199254740993.0100"),
			"values": []any{"first", false, nil, ""},
		},
	})
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
		"tenant":   common.RawJSON(`"prefix {{cfg.tenant}}"`),
		"search":   common.RawJSON(`"{{params.search}}"`),
		"enabled":  common.RawJSON(`{"$value":"params.enabled"}`),
		"number":   common.RawJSON(`{"$value":"params.number"}`),
		"repeated": common.RawJSON(`{"$value":"params.values"}`),
		"mixed":    common.RawJSON(`["{{cfg.tenant}}",{"$value":"params.enabled"},null]`),
		"null":     common.RawJSON(`null`),
		"empty":    common.RawJSON(`""`),
		"omitted":  common.RawJSON(`[]`),
	}, parameterLimits())
	require.NoError(t, err)

	// Keys sort deterministically; array entries retain their authored order.
	encoded, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "empty=&enabled=false&mixed=acme&mixed=false&mixed=null&null=null&number=9007199254740993.0100&repeated=first&repeated=false&repeated=null&repeated=&search=name+email%2Btag%26%C3%A9&tenant=prefix+acme", encoded)
}

// TestParametersLiteralNamesAndValues keeps field names and inserted text inert,
// even when they resemble Mustache expressions or reserved typed directives.
func TestParametersLiteralNamesAndValues(t *testing.T) {
	// Neither a typed string nor an explicit literal may render a second time.
	data := parameterContext(t, render.Input{
		Params: map[string]any{"text": "{{cfg.secret}}"},
		Cfg:    map[string]any{"secret": "private-configuration"},
	})
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
		"$value":         common.RawJSON(`{"$value":"params.text"}`),
		"$literal":       common.RawJSON(`{"$literal":"{{>private-file}}"}`),
		"{{cfg.secret}}": common.RawJSON(`"literal name"`),
	}, parameterLimits())
	require.NoError(t, err)

	// Encoding preserves these literal bytes without exposing the secret value.
	encoded, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "%24literal=%7B%7B%3Eprivate-file%7D%7D&%24value=%7B%7Bcfg.secret%7D%7D&%7B%7Bcfg.secret%7D%7D=literal+name", encoded)
	require.NotContains(t, encoded, "private-configuration")
}

// TestParametersRejectResolvedStructures validates the final typed value rather
// than trusting that an authored lookup resolves to a legal parameter shape.
func TestParametersRejectResolvedStructures(t *testing.T) {
	data := parameterContext(t, render.Input{
		Cfg: map[string]any{
			"object": map[string]any{"private-key": "private-configuration"},
			"nested": []any{[]any{"private-configuration"}},
			"array":  []any{"private-configuration"},
		},
	})
	for name, source := range map[string]string{
		"typed object":                `{"$value":"cfg.object"}`,
		"typed nested array":          `{"$value":"cfg.nested"}`,
		"array nested by lookup":      `[{"$value":"cfg.array"}]`,
		"authored nested array":       `[["private-configuration"]]`,
		"literal directive object":    `{"$literal":{"$value":"private-configuration"}}`,
		"object inside repeated list": `["first",{"$value":"cfg.object"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			// Compilation accepts JSON syntax independently of resolved parameter shape.
			program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
				"value": common.RawJSON(source),
			}, parameterLimits())
			require.NoError(t, err)

			// A bad value returns no partial query and never includes copied cfg data.
			encoded, err := program.Render(context.Background(), data)
			require.Error(t, err)
			require.Empty(t, encoded)
			require.NotContains(t, err.Error(), "private-configuration")
			require.NotContains(t, err.Error(), "private-key")
		})
	}
}

// TestParametersOwnSource proves source bytes and the caller's map are not kept
// as mutable references by the compiled program.
func TestParametersOwnSource(t *testing.T) {
	// Compile a source whose backing bytes and map remain accessible to the caller.
	source := common.RawJSON(`"{{params.text}}"`)
	fields := map[string]common.RawJSON{"value": source}
	program, err := httpcodec.CompileParameters(fields, parameterLimits())
	require.NoError(t, err)

	// Corrupt both input containers after compilation; the original program survives.
	source[0] = '!'
	delete(fields, "value")
	fields["replacement"] = common.RawJSON(`"changed"`)
	data := parameterContext(t, render.Input{Params: map[string]any{"text": "original"}})
	encoded, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "value=original", encoded)
}

// TestParametersConcurrentReuse keeps invocation values and output independent
// when one compiled definition serves multiple simultaneous callers.
func TestParametersConcurrentReuse(t *testing.T) {
	// The same compiled program is shared by every worker.
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
		"id":   common.RawJSON(`{"$value":"params.id"}`),
		"text": common.RawJSON(`"{{params.text}}"`),
	}, parameterLimits())
	require.NoError(t, err)
	const workers = 32
	errors := make(chan error, workers)
	var group sync.WaitGroup

	// Each worker supplies different immutable context and checks its own result.
	for i := 0; i < workers; i++ {
		data := parameterContext(t, render.Input{Params: map[string]any{
			"id": i, "text": fmt.Sprintf("value %d", i),
		}})
		group.Add(1)
		go func() {
			defer group.Done()
			encoded, err := program.Render(context.Background(), data)
			if err == nil && encoded != fmt.Sprintf("id=%d&text=value+%d", i, i) {
				err = fmt.Errorf("worker %d received unexpected parameters %q", i, encoded)
			}
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

// TestParametersCompilationLimits charges all names and values to one source
// allowance and all embedded templates to one compilation work allowance.
func TestParametersCompilationLimits(t *testing.T) {
	// Either value fits independently, but the pair exceeds the aggregate byte cap.
	limits := parameterLimits()
	limits.MaxBytes = 80
	value := common.RawJSON(`"` + strings.Repeat("x", 40) + `"`)
	_, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value}, limits)
	require.NoError(t, err)
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value, "b": value}, limits)
	require.Error(t, err)
	require.Nil(t, program)

	// Literal names consume source bytes even though they are not template values.
	limits.MaxBytes = 32
	program, err = httpcodec.CompileParameters(map[string]common.RawJSON{
		strings.Repeat("k", 32): common.RawJSON(`0`),
	}, limits)
	require.Error(t, err)
	require.Nil(t, program)

	// Multiple comment-heavy fields cannot each receive a fresh syntax work budget.
	limits = parameterLimits()
	limits.MaxSteps = 6
	comments := common.RawJSON(`"{{!one}}{{!two}}"`)
	_, err = httpcodec.CompileParameters(map[string]common.RawJSON{"a": comments}, limits)
	require.NoError(t, err)
	program, err = httpcodec.CompileParameters(map[string]common.RawJSON{"a": comments, "b": comments}, limits)
	require.Error(t, err)
	require.Nil(t, program)
}

// TestParametersRenderLimits bounds aggregate rendered data, aggregate template
// work, and percent-encoded output rather than granting each field fresh limits.
func TestParametersRenderLimits(t *testing.T) {
	t.Run("aggregate rendered bytes", func(t *testing.T) {
		// Each lookup result fits alone, while the two values exceed the shared cap.
		limits := parameterLimits()
		limits.MaxBytes = 100
		data := parameterContext(t, render.Input{Params: map[string]any{"value": strings.Repeat("x", 60)}})
		value := common.RawJSON(`{"$value":"params.value"}`)
		one, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value}, limits)
		require.NoError(t, err)
		_, err = one.Render(context.Background(), data)
		require.NoError(t, err)
		both, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value, "b": value}, limits)
		require.NoError(t, err)
		encoded, err := both.Render(context.Background(), data)
		require.Error(t, err)
		require.Empty(t, encoded)
	})

	t.Run("aggregate template work", func(t *testing.T) {
		// A short compiled section can expand into enough work to exhaust the budget.
		limits := parameterLimits()
		limits.MaxSteps = 30
		data := parameterContext(t, render.Input{Params: map[string]any{
			"items": []any{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		}})
		value := common.RawJSON(`"{{#params.items}}{{.}}{{/params.items}}"`)
		one, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value}, limits)
		require.NoError(t, err)
		_, err = one.Render(context.Background(), data)
		require.NoError(t, err)
		both, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": value, "b": value}, limits)
		require.NoError(t, err)
		encoded, err := both.Render(context.Background(), data)
		require.Error(t, err)
		require.Empty(t, encoded)
	})

	t.Run("encoded bytes include expansion", func(t *testing.T) {
		// Ten ampersands occupy thirty encoded bytes, plus the field name and equals.
		data := parameterContext(t, render.Input{})
		fields := map[string]common.RawJSON{"a": common.RawJSON(`"` + strings.Repeat("&", 10) + `"`)}
		limits := parameterLimits()
		limits.MaxBytes = 32
		program, err := httpcodec.CompileParameters(fields, limits)
		require.NoError(t, err)
		encoded, err := program.Render(context.Background(), data)
		require.NoError(t, err)
		require.Equal(t, "a="+strings.Repeat("%26", 10), encoded)

		// Reducing only the output allowance by one fails before returning any bytes.
		limits.MaxBytes--
		program, err = httpcodec.CompileParameters(fields, limits)
		require.NoError(t, err)
		encoded, err = program.Render(context.Background(), data)
		require.Error(t, err)
		require.Empty(t, encoded)
	})
}

// TestParametersInvalidSourceAndReferences rejects authoring errors at compile
// time and missing invocation data at render time without partial parameters.
func TestParametersInvalidSourceAndReferences(t *testing.T) {
	for name, source := range map[string]string{
		"invalid JSON": `{"bad":`, "multiple JSON values": `true false`,
		"partial": `"{{>local-file}}"`, "invalid directive": `{"$value":42}`,
	} {
		t.Run(name, func(t *testing.T) {
			// Parsing and template syntax errors prevent a reusable program being created.
			program, err := httpcodec.CompileParameters(map[string]common.RawJSON{"a": common.RawJSON(source)}, parameterLimits())
			require.Error(t, err)
			require.Nil(t, program)
		})
	}

	// Missing references remain strict, even after an earlier field rendered correctly.
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
		"a": common.RawJSON(`"first"`), "b": common.RawJSON(`"{{params.missing}}"`),
	}, parameterLimits())
	require.NoError(t, err)
	encoded, err := program.Render(context.Background(), parameterContext(t, render.Input{}))
	require.Error(t, err)
	require.Empty(t, encoded)
}

// TestParametersRequirePositiveLimits rejects an accidentally disabled policy,
// including for definitions that contain no parameters.
func TestParametersRequirePositiveLimits(t *testing.T) {
	for _, limits := range []render.Limits{
		{}, {MaxBytes: 0, MaxDepth: 32, MaxSteps: 1000},
		{MaxBytes: 1000, MaxDepth: 0, MaxSteps: 1000},
		{MaxBytes: 1000, MaxDepth: 32, MaxSteps: -1},
	} {
		// Empty input must not bypass validation of any policy dimension.
		program, err := httpcodec.CompileParameters(nil, limits)
		require.Error(t, err)
		require.Nil(t, program)
	}
}

// TestParametersEmptyAndCanceled handles empty definitions without special
// exceptions to the invocation's cancellation contract.
func TestParametersEmptyAndCanceled(t *testing.T) {
	data := parameterContext(t, render.Input{})
	for _, fields := range []map[string]common.RawJSON{
		nil, {}, {"a": common.RawJSON(`"value"`)},
	} {
		// Valid empty definitions produce no query or form bytes.
		program, err := httpcodec.CompileParameters(fields, parameterLimits())
		require.NoError(t, err)
		if len(fields) == 0 {
			encoded, err := program.Render(context.Background(), data)
			require.NoError(t, err)
			require.Empty(t, encoded)
		}

		// Cancellation wins even when the program would perform no template work.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		encoded, err := program.Render(ctx, data)
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, encoded)
	}
}

// TestParametersRequireProgramAndInvocation rejects incomplete callers with an
// error instead of panicking or returning apparently valid encoded parameters.
func TestParametersRequireProgramAndInvocation(t *testing.T) {
	// Use an otherwise valid program and snapshot to isolate each missing input.
	program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
		"value": common.RawJSON(`"{{params.value}}"`),
	}, parameterLimits())
	require.NoError(t, err)
	data := parameterContext(t, render.Input{Params: map[string]any{"value": "valid"}})
	for _, tc := range []struct {
		name    string
		program *httpcodec.Parameters
		ctx     context.Context
		data    *render.Context
	}{
		{"nil program", nil, context.Background(), data},
		{"zero value program", &httpcodec.Parameters{}, context.Background(), data},
		{"nil context", program, nil, data},
		{"nil snapshot", program, context.Background(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Every invalid call returns no partial result and leaves the program reusable.
			encoded, err := tc.program.Render(tc.ctx, tc.data)
			require.Error(t, err)
			require.Empty(t, encoded)
		})
	}

	// Invalid calls do not damage the shared program used by later invocations.
	encoded, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "value=valid", encoded)
}
