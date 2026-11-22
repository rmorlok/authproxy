package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/stretchr/testify/require"
)

// TestConnectionMetadataValidationUsesExecutionBudget isolates the native pass
// over decoded labels from the JSON decoder's separate conversion charges.
func TestConnectionMetadataValidationUsesExecutionBudget(t *testing.T) {
	for _, dimension := range []string{"work", "allocation", "ample"} {
		t.Run(dimension, func(t *testing.T) {
			// Prepare the native JSON graph using the standalone decoder so only
			// connection validation/defaulting consumes the ledger established below.
			labels := make(map[string]string, 64)
			for index := range 64 {
				labels[strconv.Itoa(index)] = ""
			}
			document, err := json.Marshal(map[string]any{"id": "one", "labels": labels})
			require.NoError(t, err)
			vm := goja.New()
			decoder, err := NewJSONDecoder(vm)
			require.NoError(t, err)
			values, err := decoder.Decode(context.Background(), JSONLimits{MaxBytes: 4096, MaxDepth: 4, MaxNodes: 256}, document)
			require.NoError(t, err)
			limits := Limits{MaxWorkUnits: 4096, MaxAllocationUnits: 4096}
			if dimension == "work" {
				limits.MaxWorkUnits = 64
			} else if dimension == "allocation" {
				limits.MaxAllocationUnits = 64
			}
			ctx, err := WithExecutionBudget(context.Background(), limits)
			require.NoError(t, err)

			// Empty values still require visiting and copying metadata slots.
			connection, err := connectionSnapshot(ctx, vm, values[0])
			if dimension == "ample" {
				require.NoError(t, err)
				require.NotNil(t, connection)
			} else {
				require.ErrorIs(t, err, ErrLimitExceeded)
				require.Nil(t, connection)
			}
		})
	}
}

// TestResponseEmptyHeaderValuesConsumeWork ensures empty strings cannot hide
// the variable native traversal cost of joining many response header values.
func TestResponseEmptyHeaderValuesConsumeWork(t *testing.T) {
	for _, work := range []int{16, 256} {
		limits := conversionExecutionLimits()
		limits.MaxWorkUnits = work
		err := Run(context.Background(), limits, func(ctx context.Context, vm *goja.Runtime) error {
			// No transport is needed: isolate the metadata accessor's native work.
			host, err := NewHTTPHost(ctx, vm, HTTPOptions{MaxRequestBytes: 4096, MaxPendingOperations: 1}, limits)
			if err != nil {
				return err
			}
			defer host.Close()
			response, err := host.responseValue(&aphttp.Response{Header: http.Header{"X": make([]string, 64)}})
			if err != nil {
				return err
			}
			if err := vm.Set("response", response); err != nil {
				return err
			}
			_, err = vm.RunString(`response.headers.get("X")`)
			return EvaluationError(err)
		})

		// The small work budget fails independently of returned string storage.
		if work == 16 {
			require.ErrorIs(t, err, ErrLimitExceeded)
		} else {
			require.NoError(t, err)
		}
	}
}
