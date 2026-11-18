package apjs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestBoundedOptionsCompatibility preserves supported legacy option text,
// including present undefined values, nested containers, and lexical map order.
func TestBoundedOptionsCompatibility(t *testing.T) {
	for _, source := range []string{
		`[{value:1e6,label:1e20},{value:-0,label:1e-7},{value:9007199254740991,label:1.25}]`,
		`[{value:undefined,label:null},{value:{z:undefined,a:[null,undefined,2]},label:[undefined]}]`,
		`[{value:Object.create(null),label:{"2":"b","10":"a"}},{value:true,label:false}]`,
		`[{value:"\ud800",label:"\ud801"},{value:"a\n\"b",label:"雪😀"}]`,
		`assignedWithoutDeclaration = [{value:"a",label:"Alpha"}]; assignedWithoutDeclaration;`,
		`const item = {value:"a",label:"Alpha"}; [item,item];`,
		`[]`,
	} {
		t.Run(source, func(t *testing.T) {
			// Compare only plain supported values; getters and other unsafe legacy values are excluded.
			jsctx := NewContext(nil, nil)
			legacy, err := jsctx.TransformOptions(source)
			require.NoError(t, err)
			got, err := jsctx.TransformOptionsWithContext(context.Background(), source, boundedObjectTestLimits())
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, legacy, got)
		})
	}
}

// TestBoundedOptionsNativeNumbers documents the deliberate normalization of
// reflected Go numeric inputs into JavaScript Numbers before option formatting.
func TestBoundedOptionsNativeNumbers(t *testing.T) {
	for _, tt := range []struct {
		input          any
		legacy, native string
	}{
		{float64(1e6), "1e+06", "1000000"},
		{int64(1e6), "1000000", "1000000"},
		{float64(42), "42", "42"},
		{float64(1e20), "1e+20", "1e+20"},
		{int64(9007199254740993), "9007199254740993", "9007199254740992"},
	} {
		// Identity transforms no longer retain the original host numeric type or precision.
		data := []any{map[string]any{"value": tt.input, "label": tt.input}}
		jsctx := NewContext(nil, nil)
		legacy, err := jsctx.TransformJSON("data", data)
		require.NoError(t, err)
		require.Equal(t, tt.legacy, legacy[0].Value)
		for _, source := range []string{"data", `data.map(x => ({value:x.value,label:x.label}))`} {
			got, err := jsctx.TransformJSONWithContext(context.Background(), source, data, boundedObjectTestLimits())
			require.NoError(t, err)
			require.Equal(t, []DataSourceOption{{Value: tt.native, Label: tt.native}}, got)
		}
	}
}

// TestBoundedOptionsNativeOwnership keeps input mutations local while helper
// initialization and captured conversion intrinsics retain their existing order.
func TestBoundedOptionsNativeOwnership(t *testing.T) {
	library, err := CompileLibrary(`
		let count = 0;
		const noInputs = typeof cfg === "undefined" && typeof data === "undefined";
		JSON.parse = Reflect.ownKeys = Object.getOwnPropertyDescriptor = () => { throw "private replacement"; };
	`)
	require.NoError(t, err)
	cfg := map[string]any{"items": []any{"original"}}
	labels := map[string]string{"env": "prod"}
	data := []any{map[string]any{"value": "original", "label": "Original"}}
	jsctx := library.NewContext(map[string]any{"cfg": cfg, "labels": labels, "data": "context data"})

	// Each VM receives native arrays, detached maps, no I/O host, and fresh library state.
	for range 2 {
		got, err := jsctx.TransformJSONWithContext(context.Background(), `
			cfg.items.push("local"); labels.env = "local"; data[0].value = "local";
			[{value:noInputs && Array.isArray(data) && typeof fetch === "undefined" && ++count === 1,
			  label:cfg.items}];
		`, data, boundedObjectTestLimits())
		require.NoError(t, err)
		require.Equal(t, []DataSourceOption{{Value: "true", Label: "[original local]"}}, got)
	}
	require.Equal(t, []any{"original"}, cfg["items"])
	require.Equal(t, "prod", labels["env"])
	require.Equal(t, "original", data[0].(map[string]any)["value"])

	// Binding a per-call response never replaces the reusable context's data variable.
	got, err := jsctx.TransformOptionsWithContext(context.Background(), `[{value:data,label:count}]`, boundedObjectTestLimits())
	require.NoError(t, err)
	require.Equal(t, []DataSourceOption{{Value: "context data", Label: "0"}}, got)
}

// TestBoundedOptionsConcurrentReuse isolates per-call input binding, library
// state, and formatting while callers share an immutable context and response.
func TestBoundedOptionsConcurrentReuse(t *testing.T) {
	library, err := CompileLibrary(`let count = 0;`)
	require.NoError(t, err)
	cfg := map[string]any{"items": []any{"original"}}
	data := []any{map[string]any{"label": "Original"}}
	jsctx := library.NewContext(map[string]any{"cfg": cfg, "data": "context data"})
	results, start := make(chan error, 8), make(chan struct{})

	// Concurrent mutations must affect only each invocation's copied native graph.
	for range cap(results) {
		go func() {
			<-start
			got, err := jsctx.TransformJSONWithContext(context.Background(), `
				cfg.items.push("local"); data[0].label = "local";
				[{value:{count:++count,items:cfg.items},label:data[0].label}];
			`, data, boundedObjectTestLimits())
			want := DataSourceOption{Value: "map[count:1 items:[original local]]", Label: "local"}
			if err == nil && (len(got) != 1 || got[0] != want) {
				err = fmt.Errorf("unexpected concurrent options: %+v", got)
			}
			results <- err
		}()
	}
	close(start)
	for range cap(results) {
		require.NoError(t, <-results)
	}

	// Neither the shared response nor the reusable context acquired local changes.
	require.Equal(t, []any{"original"}, cfg["items"])
	require.Equal(t, "Original", data[0].(map[string]any)["label"])
	require.Equal(t, "context data", jsctx.vars["data"])
}

// TestBoundedOptionsFailures rejects unsafe graphs and malformed option shapes
// without invoking authored accessors or returning partial options/private text.
func TestBoundedOptionsFailures(t *testing.T) {
	for name, source := range map[string]string{
		"undefined root": `undefined`, "null root": `null`, "object root": `({value:1,label:2})`,
		"scalar item": `["privateDetail"]`, "missing value": `[{label:"privateDetail"}]`,
		"missing label": `[{value:"privateDetail"}]`, "partial result": `[{value:1,label:2},{}]`,
		"getter":    `[{get value() { for (;;) {} },label:"privateDetail"}]`,
		"proxy":     `new Proxy([],{ownKeys() { for (;;) {} }})`,
		"cycle":     `const value = {}; value.self = value; [{value,label:1}]`,
		"prototype": `[{value:Object.create({privateDetail:1}),label:1}]`,
		"function":  `[{value:1,label:2,privateDetail() {}}]`,
		"symbol":    `[{value:Symbol("privateDetail"),label:1}]`,
		"bigint":    `[{value:1n,label:1}]`, "nonfinite": `[{value:NaN,label:Infinity}]`,
		"boxed": `[{value:new Number(1),label:1}]`, "sparse": `[{value:[,1],label:1}]`,
	} {
		t.Run(name, func(t *testing.T) {
			// Deliberate loops in getters and proxies must be rejected, not executed until timeout.
			got, err := NewContext(nil, nil).TransformOptionsWithContext(context.Background(), source, boundedObjectTestLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
			require.Nil(t, got)
			require.NotContains(t, err.Error(), "privateDetail")
			diagnostic, ok := Diagnostic(err)
			require.True(t, ok)
			require.Equal(t, "output", diagnostic.Phase)
		})
	}
}

// TestBoundedOptionsFinalBytes checks the complete serialized response, whose
// escaped strings and fmt-style containers can exceed the original JSON result.
func TestBoundedOptionsFinalBytes(t *testing.T) {
	for _, tt := range []struct {
		source, intermediate string
		want                 []DataSourceOption
	}{
		{`[{value:"x",label:"y"}]`, `[{"value":"x","label":"y"}]`, []DataSourceOption{{Value: "x", Label: "y"}}},
		{`[{value:"<>&\u2028",label:null}]`, "[{\"value\":\"<>&\u2028\",\"label\":null}]", []DataSourceOption{{Value: "<>&\u2028", Label: "<nil>"}}},
		{`[{value:[{},{},null],label:{a:null,b:{}}}]`, `[{"value":[{},{},null],"label":{"a":null,"b":{}}}]`, []DataSourceOption{{Value: "[map[] map[] <nil>]", Label: "map[a:<nil> b:map[]]"}}},
	} {
		// The exact final JSON fits; reducing its budget by one byte discards all options.
		encoded, err := json.Marshal(tt.want)
		require.NoError(t, err)
		limits := boundedObjectTestLimits()
		limits.MaxOutputBytes = len(encoded)
		got, err := NewContext(nil, nil).TransformOptionsWithContext(context.Background(), tt.source, limits)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
		limits.MaxOutputBytes--
		got, err = NewContext(nil, nil).TransformOptionsWithContext(context.Background(), tt.source, limits)
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.Nil(t, got)

		// Fitting the authored JSON alone is insufficient when formatting/escaping expands it.
		if len(encoded) > len(tt.intermediate) {
			limits.MaxOutputBytes = len(tt.intermediate)
			got, err = NewContext(nil, nil).TransformOptionsWithContext(context.Background(), tt.source, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, got)
		}
	}
}

// TestBoundedOptionsLimits covers source admission, native input copying,
// traversal, execution, and safe authored-error classification.
func TestBoundedOptionsLimits(t *testing.T) {
	for name, tt := range map[string]struct {
		source, library string
		vars            map[string]any
		limit           func(*jsruntime.Limits)
		want            error
	}{
		"source":            {source: `[]`, limit: func(l *jsruntime.Limits) { l.MaxSourceBytes = 1 }},
		"combined source":   {library: `function helper() {}`, source: `[]`, limit: func(l *jsruntime.Limits) { l.MaxSourceBytes = len(`function helper() {}`) + 1 }},
		"input bytes":       {source: `[]`, vars: map[string]any{"padding": strings.Repeat("x", 100)}, limit: func(l *jsruntime.Limits) { l.MaxInputBytes = 16 }},
		"input graph":       {source: `[]`, vars: map[string]any{"cfg": map[string]any{"items": []any{1, 2, 3}}}, limit: func(l *jsruntime.Limits) { l.MaxNodes = 4 }},
		"output nodes":      {source: `[{value:1,label:2}]`, limit: func(l *jsruntime.Limits) { l.MaxNodes = 5 }},
		"output depth":      {source: `[{value:1,label:2}]`, limit: func(l *jsruntime.Limits) { l.MaxDepth = 1 }},
		"stack":             {source: `function recurse() { return recurse(); } recurse();`, limit: func(l *jsruntime.Limits) { l.MaxStackDepth = 16 }},
		"execution":         {source: `for (;;) {}`, limit: func(l *jsruntime.Limits) { l.Timeout = 25 * time.Millisecond }},
		"initialization":    {library: `for (;;) {}`, source: `[]`, limit: func(l *jsruntime.Limits) { l.Timeout = 25 * time.Millisecond }},
		"syntax":            {source: `const privateDetail = ;`, want: jsruntime.ErrInvalidSource},
		"throw":             {source: `throw "privateDetail"`, want: jsruntime.ErrEvaluation},
		"unsupported input": {source: `[]`, vars: map[string]any{"privateDetail": make(chan int)}, want: jsruntime.ErrInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			limits := boundedObjectTestLimits()
			if tt.limit != nil {
				tt.limit(&limits)
			}
			if tt.want == nil {
				tt.want = jsruntime.ErrLimitExceeded
			}
			library, err := CompileLibrary(tt.library)
			require.NoError(t, err)

			// Failures retain their shared category but never expose authored details or partial results.
			got, err := library.NewContext(tt.vars).TransformOptionsWithContext(context.Background(), tt.source, limits)
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, got)
			require.NotContains(t, err.Error(), "privateDetail")
		})
	}
}

// TestBoundedOptionsCancellation keeps caller errors distinct from policy
// timeouts and stops detached conversion before publishing any partial result.
func TestBoundedOptionsCancellation(t *testing.T) {
	jsctx := NewContext(nil, nil)
	limits := boundedObjectTestLimits()
	got, err := jsctx.TransformOptionsWithContext(nil, `[]`, limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, got)
	invalid := limits
	invalid.MaxOutputBytes = 0
	got, err = jsctx.TransformOptionsWithContext(context.Background(), `[]`, invalid)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, got)

	// An already-canceled caller wins over invalid source/policy and detached output work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = jsctx.TransformOptionsWithContext(ctx, `privateDetail ===`, invalid)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
	got, err = dataSourceOptionsFromJSON(ctx, json.RawMessage(`[{"value":1,"label":2}]`), limits)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)

	// A shorter request deadline interrupts JavaScript and keeps the original context cause.
	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	got, err = jsctx.TransformOptionsWithContext(ctx, `for (;;) {}`, limits)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, got)
}
