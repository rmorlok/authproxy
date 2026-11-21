package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// conversionExecutionLimits separates generous boundary caps from the shared
// work/allocation limits tightened by each conversion scenario.
func conversionExecutionLimits() Limits {
	return Limits{
		MaxSourceBytes: 4096, MaxInputBytes: 4096, MaxOutputBytes: 4096,
		MaxDepth: 16, MaxNodes: 256, MaxStackDepth: 128, Timeout: time.Second,
		MaxWorkUnits: 4096, MaxAllocationUnits: 4096,
	}
}

// TestConversionAndUtilityShareExecutionBudget makes native input, a pure
// helper, and final output consume one execution-owned allowance.
func TestConversionAndUtilityShareExecutionBudget(t *testing.T) {
	for _, test := range []struct {
		name             string
		work, allocation int
		fail             bool
	}{
		{"exact aggregate", 28, 18, false},
		{"work exhausted by final conversion", 27, 18, true},
		{"allocation exhausted by final conversion", 28, 17, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := conversionExecutionLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = test.work, test.allocation
			limits.MaxInputBytes, limits.MaxOutputBytes = 3, 6
			result, err := EvaluateJSON(context.Background(), limits, func(ctx context.Context, vm *goja.Runtime) (goja.Value, error) {
				decoder, err := NewJSONDecoder(vm)
				if err != nil {
					return nil, err
				}
				// Three-byte JSON input consumes 14 work and eight allocation units.
				values, err := decoder.Decode(ctx, JSONLimits{MaxBytes: 3, MaxDepth: 1, MaxNodes: 1}, json.RawMessage(`"a"`))
				if err != nil {
					return nil, err
				}
				encode, ok := goja.AssertFunction(vm.Get("btoa"))
				require.True(t, ok)
				// The helper adds two work/four allocation units. Encoding "YQ=="
				// as final JSON adds twelve work/six allocation units afterward.
				value, err := encode(goja.Undefined(), values[0])
				return value, EvaluationError(err)
			})

			if test.fail {
				require.ErrorIs(t, err, ErrLimitExceeded)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `"YQ=="`, string(result))
			}
		})
	}
}

// TestRepeatedJSONDecodeKeepsExecutionBudget permits fresh document boundaries
// without replenishing cumulative work or storage on later Decode calls.
func TestRepeatedJSONDecodeKeepsExecutionBudget(t *testing.T) {
	for _, test := range []struct {
		name             string
		work, allocation int
		fail             bool
	}{
		{"exact two decodes", 12, 8, false},
		{"shared work", 11, 8, true},
		{"shared storage", 12, 7, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := conversionExecutionLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = test.work, test.allocation
			err := Run(context.Background(), limits, func(ctx context.Context, vm *goja.Runtime) error {
				decoder, err := NewJSONDecoder(vm)
				if err != nil {
					return err
				}
				// Each single-value document independently fits the one-byte/node boundary.
				for range 2 {
					_, err := decoder.Decode(ctx, JSONLimits{MaxBytes: 1, MaxDepth: 1, MaxNodes: 1}, json.RawMessage(`1`))
					if err != nil {
						return err
					}
				}
				return nil
			})
			if test.fail {
				require.ErrorIs(t, err, ErrLimitExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestHiddenOutputMetadataConsumesWork charges inspected keys even when their
// values are omitted and the final JSON remains only two bytes.
func TestHiddenOutputMetadataConsumesWork(t *testing.T) {
	for _, work := range []int{32, 4096} {
		limits := conversionExecutionLimits()
		limits.MaxWorkUnits, limits.MaxOutputBytes = work, 2
		result, err := EvaluateJSON(context.Background(), limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
			// A long non-enumerable name produces metadata work but no output field.
			value, err := vm.RunString(`Object.defineProperty({}, "x".repeat(128), {value: true})`)
			return value, EvaluationError(err)
		})
		if work == 32 {
			require.ErrorIs(t, err, ErrLimitExceeded)
			require.Nil(t, result)
		} else {
			require.NoError(t, err)
			require.Equal(t, `{}`, string(result))
		}
	}
}
