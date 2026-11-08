package apjs

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// variableTestLimits allows small connector fixtures while keeping every
// conversion dimension finite and independently adjustable in boundary tests.
func variableTestLimits() jsruntime.JSONLimits {
	return jsruntime.JSONLimits{MaxBytes: 4096, MaxDepth: 16, MaxNodes: 1000}
}

// TestEncodeVariablesJSONValues covers ordinary connector configuration and
// metadata containers without exposing any reflected Go objects to the VM.
func TestEncodeVariablesJSONValues(t *testing.T) {
	vars := map[string]any{
		"cfg": map[string]any{
			"null": nil, "enabled": true, "region": "café 世界 😀",
			"numbers": []any{
				int(-1), int8(-2), int16(-3), int32(-4), int64(-5),
				uint(1), uint8(2), uint16(3), uint32(4), uint64(5),
				float32(1.25), float64(2.5), json.Number("1e3"),
			},
			"names": []string{"first", "second"},
		},
		"labels":      map[string]string{"environment": "test"},
		"annotations": map[string]string{},
	}

	// The restricted serializer preserves the JSON meaning of each supported Go value.
	encoded, err := encodeVariables(context.Background(), vars, variableTestLimits())
	require.NoError(t, err)
	expected, err := json.Marshal(vars)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(encoded))

	// The shared decoder accepts the entire snapshot as native JavaScript data.
	decoder, err := jsruntime.NewJSONDecoder(goja.New())
	require.NoError(t, err)
	values, err := decoder.Decode(context.Background(), variableTestLimits(), encoded)
	require.NoError(t, err)
	require.Len(t, values, 1)
}

// TestEncodeVariablesNilContainers distinguishes absent globals from explicit
// nested null values, preserving the JSON contract used by configuration data.
func TestEncodeVariablesNilContainers(t *testing.T) {
	// No variables still produces an object suitable for installing zero globals.
	encoded, err := encodeVariables(context.Background(), nil, variableTestLimits())
	require.NoError(t, err)
	require.Equal(t, `{}`, string(encoded))

	// Typed nil containers inside a supplied variable retain null rather than becoming empty.
	encoded, err = encodeVariables(context.Background(), map[string]any{
		"map": map[string]any(nil), "labels": map[string]string(nil),
		"array": []any(nil), "names": []string(nil),
	}, variableTestLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"map":null,"labels":null,"array":null,"names":null}`, string(encoded))
}

// TestEncodeVariablesLimits checks byte, node, and root-zero depth boundaries
// against the shared JSON decoder's accounting rather than estimating from Go sizes.
func TestEncodeVariablesLimits(t *testing.T) {
	vars := map[string]any{"cfg": map[string]any{"items": []any{true}}}
	const expected = `{"cfg":{"items":[true]}}`
	limits := jsruntime.JSONLimits{MaxBytes: len(expected), MaxDepth: 3, MaxNodes: 6}

	// This snapshot fits exactly: root, two keys, two containers, and one Boolean.
	encoded, err := encodeVariables(context.Background(), vars, limits)
	require.NoError(t, err)
	require.Equal(t, expected, string(encoded))
	decoder, err := jsruntime.NewJSONDecoder(goja.New())
	require.NoError(t, err)
	_, err = decoder.Decode(context.Background(), limits, encoded)
	require.NoError(t, err)

	for name, change := range map[string]func(*jsruntime.JSONLimits){
		"bytes": func(l *jsruntime.JSONLimits) { l.MaxBytes-- },
		"depth": func(l *jsruntime.JSONLimits) { l.MaxDepth-- },
		"nodes": func(l *jsruntime.JSONLimits) { l.MaxNodes-- },
	} {
		t.Run(name, func(t *testing.T) {
			// Tightening any dimension by one rejects the whole document without partial output.
			tighter := limits
			change(&tighter)
			result, err := encodeVariables(context.Background(), vars, tighter)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestEncodeVariablesEscapedBytes accounts for quotes and JSON escapes before
// accepting the final document, even when the raw string itself would fit.
func TestEncodeVariablesEscapedBytes(t *testing.T) {
	vars := map[string]any{"text": "\n\u0000\"\\"}
	expected, err := json.Marshal(vars)
	require.NoError(t, err)
	limits := variableTestLimits()
	limits.MaxBytes = len(expected)

	// Exact serialized bytes include both object syntax and escaped string contents.
	encoded, err := encodeVariables(context.Background(), vars, limits)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(encoded))
	limits.MaxBytes--
	encoded, err = encodeVariables(context.Background(), vars, limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, encoded)

	// Raw strings and numeric spellings are also rejected before oversized scalar marshaling.
	for _, value := range []any{strings.Repeat("x", 128), json.Number(strings.Repeat("1", 128))} {
		limits.MaxBytes = 16
		encoded, err = encodeVariables(context.Background(), map[string]any{"x": value}, limits)
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.Nil(t, encoded)
	}
}

// variableMarshaler is an unsupported host object whose method must never run.
type variableMarshaler struct{ called *bool }

// MarshalJSON marks an attempted custom conversion so the test can reject it.
func (m variableMarshaler) MarshalJSON() ([]byte, error) {
	*m.called = true
	return []byte(`"private host value"`), nil
}

// TestEncodeVariablesRejectsHostValues prevents custom conversion hooks or live
// Go values from entering the bounded/native input path.
func TestEncodeVariablesRejectsHostValues(t *testing.T) {
	called := false
	for _, value := range []any{
		variableMarshaler{called: &called}, &variableMarshaler{called: &called},
		struct{ Private string }{"private host value"}, func() {}, make(chan int),
		new(string), map[int]string{1: "private"}, []byte("private"), json.RawMessage(`{}`),
	} {
		// Only the declared JSON containers and primitive types are accepted; no hooks run.
		encoded, err := encodeVariables(context.Background(), map[string]any{"x": value}, variableTestLimits())
		require.Equal(t, jsruntime.ErrInvalidInput, err)
		require.Nil(t, encoded)
		require.False(t, called)
	}
}

// TestEncodeVariablesRejectsInvalidJSON checks values that ordinary encoding
// might replace or that cannot be represented as finite JavaScript numbers.
func TestEncodeVariablesRejectsInvalidJSON(t *testing.T) {
	for _, vars := range []map[string]any{
		{"x": "\xff"}, {"\xff": true},
		{"x": math.NaN()}, {"x": math.Inf(1)}, {"x": float32(math.Inf(-1))},
		{"x": json.Number("1e9999")}, {"x": json.Number("01")}, {"x": json.Number("private")},
	} {
		// Invalid UTF-8, numeric grammar, and non-finite values fail with fixed diagnostics.
		encoded, err := encodeVariables(context.Background(), vars, variableTestLimits())
		require.Equal(t, jsruntime.ErrInvalidInput, err)
		require.Nil(t, encoded)
	}
}

// TestEncodeVariablesDetachedOutput confirms that later caller changes cannot
// mutate a completed snapshot or the native values decoded from it.
func TestEncodeVariablesDetachedOutput(t *testing.T) {
	items := []any{"original"}
	labels := map[string]string{"environment": "test"}
	vars := map[string]any{"cfg": map[string]any{"items": items}, "labels": labels}
	encoded, err := encodeVariables(context.Background(), vars, variableTestLimits())
	require.NoError(t, err)

	// Mutating the original maps and slices after encoding does not edit the JSON buffer.
	items[0] = "changed"
	labels["environment"] = "changed"
	vars["cfg"] = nil
	require.JSONEq(t, `{"cfg":{"items":["original"]},"labels":{"environment":"test"}}`, string(encoded))
}

// TestEncodeVariablesCancellationAndCycles rejects canceled work and stops
// malformed recursive graphs at a finite traversal bound without retaining them.
func TestEncodeVariablesCancellationAndCycles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A canceled caller receives its context error before even an empty snapshot is copied.
	encoded, err := encodeVariables(ctx, nil, variableTestLimits())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, encoded)

	// Both cyclic maps and slices stop without unbounded reflection or serialization.
	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := []any{nil}
	cyclicSlice[0] = cyclicSlice
	for _, value := range []any{cyclicMap, cyclicSlice} {
		encoded, err = encodeVariables(context.Background(), map[string]any{"x": value}, variableTestLimits())
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.Nil(t, encoded)
	}
}

// TestEncodeVariablesInvalidPolicy rejects missing context or disabled caps
// before starting conversion; callers must always supply a finite policy.
func TestEncodeVariablesInvalidPolicy(t *testing.T) {
	encoded, err := encodeVariables(nil, nil, variableTestLimits())
	require.Equal(t, jsruntime.ErrInvalidInput, err)
	require.Nil(t, encoded)
	for _, limits := range []jsruntime.JSONLimits{
		{}, {MaxBytes: 1, MaxDepth: 1}, {MaxBytes: 1, MaxNodes: 1}, {MaxDepth: 1, MaxNodes: 1},
	} {
		// An absent limit never means unlimited execution.
		encoded, err := encodeVariables(context.Background(), nil, limits)
		require.Equal(t, jsruntime.ErrInvalidInput, err)
		require.Nil(t, encoded)
	}
}
