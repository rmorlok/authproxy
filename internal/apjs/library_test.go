package apjs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
