package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestValidateJSONStrings preserves representable strings and rejects escaped
// lone units before native decoding can silently replace authored text or keys.
func TestValidateJSONStrings(t *testing.T) {
	for _, test := range []struct {
		name    string
		encoded string
		invalid bool
	}{
		{"nonstring root", `false`, false},
		{"unicode", `{"text":"é😀"}`, false},
		{"paired units", `"\uD83D\uDE00"`, false},
		{"paired key", `{"\ud83d\ude00":"value"}`, false},
		{"literal escape", `"\\ud800"`, false},
		{"literal key", `{"\\udfff":"value"}`, false},
		{"escaped quote", `{"text":"quoted\" text"}`, false},
		{"high unit", `"\ud800"`, true},
		{"low unit", `"\udfff"`, true},
		{"high key", `{"\ud800":"value"}`, true},
		{"low key", `{"\udc00":"value"}`, true},
		{"unpaired before text", `"\ud800x"`, true},
		{"high followed by ordinary escape", `"\ud800\u0041"`, true},
		{"high followed by literal escape", `"\ud800\\udc00"`, true},
		{"lone after pair", `"\ud83d\ude00\udfff"`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Syntax is a caller precondition; the helper only checks string fidelity.
			encoded := []byte(test.encoded)
			require.True(t, json.Valid(encoded))
			err := jsruntime.ValidateJSONStrings(context.Background(), encoded)
			if test.invalid {
				require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
				require.Equal(t, jsruntime.ErrInvalidInput.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateJSONStringsCancellation preserves caller cancellation ahead of
// invalid-string classification and rejects a missing execution context safely.
func TestValidateJSONStringsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Canceled work does not inspect the authored string or replace its cause.
	require.ErrorIs(t, jsruntime.ValidateJSONStrings(ctx, []byte(`"\ud800"`)), context.Canceled)
	require.ErrorIs(t, jsruntime.ValidateJSONStrings(nil, []byte(`"text"`)), jsruntime.ErrInvalidInput)
}

// TestValidateJSONStringsSharesExecutionBudget charges repeated scans to the
// existing execution ledger and preserves exhaustion for subsequent phases.
func TestValidateJSONStringsSharesExecutionBudget(t *testing.T) {
	encoded := []byte(`{"text":"hello"}`)
	ctx, err := jsruntime.WithExecutionBudget(context.Background(), jsruntime.Limits{
		MaxWorkUnits: 2 * len(encoded), MaxAllocationUnits: 1,
	})
	require.NoError(t, err)

	// One prior native phase and this scan together use the exact allowance.
	require.NoError(t, jsruntime.ChargeWork(ctx, len(encoded)))
	require.NoError(t, jsruntime.ValidateJSONStrings(ctx, encoded))
	require.NoError(t, jsruntime.ChargeWork(ctx, 0))

	// A second scan cannot reset the allowance, and the failure stays terminal.
	require.ErrorIs(t, jsruntime.ValidateJSONStrings(ctx, encoded), jsruntime.ErrLimitExceeded)
	require.ErrorIs(t, jsruntime.ValidateJSONStrings(ctx, []byte(`"\ud800"`)), jsruntime.ErrLimitExceeded)
	require.ErrorIs(t, jsruntime.ReserveAllocation(ctx, 0), jsruntime.ErrLimitExceeded)
}
