package apjs

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryParsingDoesNotLoadSourceMaps(t *testing.T) {
	// The default parser fails on this missing file, so success proves it was never read.
	const directive = `\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json`
	sourceMap := "\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"

	// Static library and expression parsing ignore the directive.
	library, err := CompileLibrary("const value = 7;" + sourceMap)
	require.NoError(t, err)
	require.NoError(t, ValidateExpressionSyntax("1 + 1;"+sourceMap))

	// Dynamic parsing during library validation uses the same policy.
	dynamic, err := CompileLibrary(`eval("1` + directive + `");`)
	require.NoError(t, err)
	require.NoError(t, dynamic.Validate())

	// Expression source and its dynamic parsing also stay away from the filesystem.
	jsctx := library.NewContext(nil)
	matched, err := jsctx.EvaluateBoolean("value === 7;" + sourceMap)
	require.NoError(t, err)
	assert.True(t, matched)
	matched, err = jsctx.EvaluateBoolean(`eval("value === 7` + directive + `")`)
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestLibraryContextEvaluateBoolean(t *testing.T) {
	library, err := CompileAndValidateLibrary(`
		const PERSONAL_CALENDAR = "yes";

		function isPersonalCalendar(cfg) {
			return cfg.calendar_id === PERSONAL_CALENDAR;
		}
	`)
	require.NoError(t, err)

	ctx := library.NewContext(map[string]any{
		"cfg": map[string]any{
			"calendar_id": "yes",
		},
		"labels":      map[string]string{"env": "prod"},
		"annotations": map[string]string{},
	})

	result, err := ctx.EvaluateBoolean(`isPersonalCalendar(cfg) && labels.env === "prod"`)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestLibraryContextTransformOptions(t *testing.T) {
	library, err := CompileAndValidateLibrary(`
		function transformCalendarItems(items, prefix) {
			return items.map(function(c) {
				return { value: prefix + c.id, label: c.summary || c.id };
			});
		}
	`)
	require.NoError(t, err)

	ctx := library.NewContext(map[string]any{
		"cfg": map[string]any{
			"value_prefix": "cal:",
		},
		"labels":      map[string]string{},
		"annotations": map[string]string{},
		"data": map[string]any{
			"items": []any{
				map[string]any{"id": "primary", "summary": "Primary"},
				map[string]any{"id": "team"},
			},
		},
	})

	options, err := ctx.TransformOptions(`transformCalendarItems(data.items, cfg.value_prefix)`)
	require.NoError(t, err)
	require.Len(t, options, 2)
	assert.Equal(t, DataSourceOption{Value: "cal:primary", Label: "Primary"}, options[0])
	assert.Equal(t, DataSourceOption{Value: "cal:team", Label: "team"}, options[1])
}

func TestLibraryContextTransformJSON(t *testing.T) {
	library, err := CompileAndValidateLibrary(`
		function toOptions(items) {
			return items.map(function(item) {
				return { value: item.id, label: item.name };
			});
		}
	`)
	require.NoError(t, err)

	options, err := library.NewContext(map[string]any{
		"cfg":         map[string]any{},
		"labels":      map[string]string{},
		"annotations": map[string]string{},
	}).TransformJSON(`toOptions(data)`, []any{
		map[string]any{"id": "a", "name": "Alpha"},
	})
	require.NoError(t, err)
	require.Len(t, options, 1)
	assert.Equal(t, DataSourceOption{Value: "a", Label: "Alpha"}, options[0])
}

func TestCompileLibraryRejectsSyntaxError(t *testing.T) {
	_, err := CompileLibrary(`function broken(`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile connector JavaScript library")
}

func TestCompileAndValidateLibraryRejectsTopLevelRuntimeError(t *testing.T) {
	_, err := CompileAndValidateLibrary(`throw new Error("boom")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JS library error")
	assert.Contains(t, err.Error(), "boom")
}

func TestCompileAndValidateLibraryRunsWithoutRuntimeVars(t *testing.T) {
	_, err := CompileAndValidateLibrary(`var selected = cfg.calendar_id`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JS library error")
}

func TestLibraryRejectsReservedRuntimeVars(t *testing.T) {
	_, err := CompileAndValidateLibrary(`const cfg = { calendar_id: "yes" }`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `reserved runtime variable "cfg"`)

	library, err := CompileLibrary(`const data = []`)
	require.NoError(t, err)

	_, err = library.NewContext(map[string]any{"data": []any{}}).TransformOptions(`data`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `reserved runtime variable "data"`)
}

func TestLibraryEvaluationUsesFreshRuntime(t *testing.T) {
	library, err := CompileAndValidateLibrary(`
		var count = 0;
		function firstCallOnly() {
			count++;
			return count === 1;
		}
	`)
	require.NoError(t, err)

	ctx := library.NewContext(nil)

	first, err := ctx.EvaluateBoolean(`firstCallOnly()`)
	require.NoError(t, err)
	assert.True(t, first)

	second, err := ctx.EvaluateBoolean(`firstCallOnly()`)
	require.NoError(t, err)
	assert.True(t, second)
}

func TestLibraryRunInitializesCallerRuntime(t *testing.T) {
	// Compile once, with lexical state and a helper that uses caller-provided data.
	library, err := CompileLibrary(`
		const PREFIX = suppliedPrefix;
		let count = 0;
		function nextItem() { return PREFIX + (++count); }
	`)
	require.NoError(t, err)

	// Initialization leaves lexical names and helper functions available to later code.
	first := goja.New()
	require.NoError(t, first.Set("suppliedPrefix", "first:"))
	require.NoError(t, library.Run(first))
	value, err := first.RunString(`PREFIX + "/" + nextItem() + "/" + nextItem()`)
	require.NoError(t, err)
	assert.Equal(t, "first:/first:1/first:2", value.String())

	// Reusing the compiled library in another runtime creates independent lexical state.
	second := goja.New()
	require.NoError(t, second.Set("suppliedPrefix", "second:"))
	require.NoError(t, library.Run(second))
	value, err = second.RunString(`nextItem()`)
	require.NoError(t, err)
	assert.Equal(t, "second:1", value.String())

	// Library initialization adds no network, module-loading, or process capability.
	value, err = first.RunString(`typeof fetch + "/" + typeof require + "/" + typeof process`)
	require.NoError(t, err)
	assert.Equal(t, "undefined/undefined/undefined", value.String())
}

func TestLibraryRunLeavesReservedNamePolicyToCaller(t *testing.T) {
	// A different JavaScript host may reserve names other than connector predicates do.
	library, err := CompileLibrary(`const cfg = { tenant: "example" };`)
	require.NoError(t, err)
	vm := goja.New()
	require.NoError(t, library.Run(vm))
	value, err := vm.RunString(`cfg.tenant`)
	require.NoError(t, err)
	assert.Equal(t, "example", value.String())

	// The existing connector validation path continues to enforce its own reserved names.
	require.ErrorContains(t, library.Validate(), `reserved runtime variable "cfg"`)
}

func TestLibraryRunEmptyLibraries(t *testing.T) {
	// Missing, zero-value, and whitespace-only libraries all preserve the caller's VM.
	empty, err := CompileLibrary(" \n\t")
	require.NoError(t, err)
	for name, library := range map[string]*Library{
		"nil":        nil,
		"zero value": {},
		"empty":      empty,
	} {
		t.Run(name, func(t *testing.T) {
			vm := goja.New()
			require.NoError(t, vm.Set("existing", "preserved"))
			require.NoError(t, library.Run(vm))
			assert.Equal(t, "preserved", vm.Get("existing").String())
		})
	}
}

func TestLibraryRunRequiresRuntime(t *testing.T) {
	// Reject missing runtimes consistently, including when there is no source to execute.
	library, err := CompileLibrary(`const value = 1;`)
	require.NoError(t, err)
	for name, library := range map[string]*Library{
		"nil":        nil,
		"zero value": {},
		"compiled":   library,
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, library.Run(nil), "runtime is required")
		})
	}
}
