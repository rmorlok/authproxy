package runtime_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestLoggingRecordBoundaries applies one serialized-byte/node allowance to
// the message and fields together, with ordinary depth bounds on each value.
// A refusal is terminal and cannot publish a partially converted record.
func TestLoggingRecordBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, call string
		limit      func(*jsruntime.Limits, bool)
	}{
		{
			"combined bytes", `console.info("ok", {n: 1});`,
			func(limits *jsruntime.Limits, fits bool) {
				// The quoted message uses 4 bytes and {"n":1} uses 7: both fit
				// separately under 10, but the combined record needs 11.
				limits.MaxOutputBytes = 10
				if fits {
					limits.MaxOutputBytes++
				}
			},
		},
		{
			"depth", `console.info("ok", {nested: {n: 1}});`,
			func(limits *jsruntime.Limits, fits bool) {
				// The fields root is depth 0, its nested object 1, and n is 2.
				limits.MaxDepth = 1
				if fits {
					limits.MaxDepth++
				}
			},
		},
		{
			"combined nodes", `console.info("ok", {n: 1});`,
			func(limits *jsruntime.Limits, fits bool) {
				// Message, fields object, property inspection, and numeric value
				// are four nodes; a fresh allowance for fields would miss this.
				limits.MaxNodes = 3
				if fits {
					limits.MaxNodes++
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, fits := range []bool{true, false} {
				limits := runtimeLimits()
				test.limit(&limits, fits)
				var buffer bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&buffer, nil))
				program, err := jsruntime.CompileAsync(`async function execute() {
					console.info("kept");
					try {`+test.call+`} catch (_) {}
					console.info("late");
					return null;
				}`, nil, nil, limits)
				require.NoError(t, err)
				result, err := program.Evaluate(context.Background(), loggingInput(logger))

				if fits {
					// Every record has fresh boundary caps and the exact limit admits it.
					require.NoError(t, err)
					require.JSONEq(t, `null`, string(result))
					require.Len(t, loggingRecords(t, &buffer), 3)
				} else {
					// Catching the failed record cannot permit a later effect or result.
					require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
					require.Nil(t, result)
					records := loggingRecords(t, &buffer)
					require.Len(t, records, 1)
					require.Equal(t, "kept", records[0]["javascript"].(map[string]any)["message"])
				}
			}
		})
	}
}

// TestLoggingCumulativeBudgetIncludesFilteredRecords keeps shared execution
// accounting independent of record boundaries and the sink's level filter.
func TestLoggingCumulativeBudgetIncludesFilteredRecords(t *testing.T) {
	source := `async function execute() {
		console.error("kept");
		try {for (let i = 0; i < 1000; i++) {console.info("repeated");}} catch (_) {}
		console.error("late");
		return null;
	}`
	for _, dimension := range []string{"work", "allocation"} {
		for _, level := range []slog.Level{slog.LevelInfo, slog.LevelError} {
			t.Run(dimension+"/"+level.String(), func(t *testing.T) {
				// Every individual record fits; only the cumulative dimension is small.
				limits := runtimeLimits()
				limits.Timeout = 5 * time.Second
				if dimension == "work" {
					limits.MaxWorkUnits = 1024
				} else {
					limits.MaxAllocationUnits = 1024
				}
				var buffer bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: level}))
				program, err := jsruntime.CompileAsync(source, nil, nil, limits)
				require.NoError(t, err)
				result, err := program.Evaluate(context.Background(), loggingInput(logger))

				// Earlier records survive, but no caught refusal can emit the last record.
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
				require.Nil(t, result)
				records := loggingRecords(t, &buffer)
				require.NotEmpty(t, records)
				require.Less(t, len(records), 1002)
				require.Equal(t, "kept", records[0]["javascript"].(map[string]any)["message"])
				for _, record := range records {
					require.NotEqual(t, "late", record["javascript"].(map[string]any)["message"])
				}
				if level == slog.LevelError {
					require.Len(t, records, 1, "filtered calls must still exhaust the budget")
				}
			})
		}
	}

	// The same work completes under an ordinary execution allowance.
	var buffer bytes.Buffer
	program := compileAsync(t, source, nil)
	result, err := program.Evaluate(context.Background(), loggingInput(slog.New(slog.NewJSONHandler(&buffer, nil))))
	require.NoError(t, err)
	require.JSONEq(t, `null`, string(result))
	require.Len(t, loggingRecords(t, &buffer), 1002)
}

// TestLoggingExhaustionStopsHTTP preserves both earlier log records and target
// send evidence while preventing later logging or HTTP after shared exhaustion.
func TestLoggingExhaustionStopsHTTP(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		t.Run(dimension, func(t *testing.T) {
			// The first log and request fit; a later message exceeds the remaining ledger.
			limits := runtimeLimits()
			if dimension == "work" {
				limits.MaxWorkUnits = 8192
			} else {
				limits.MaxAllocationUnits = 8192
			}
			program, err := jsruntime.CompileAsync(`async function execute() {
				console.info("kept");
				await fetch("https://example.test/first");
				try {console.info("x".repeat(8192));} catch (_) {}
				console.info("late");
				await fetch("https://example.test/second");
				return null;
			}`, nil, nil, limits)
			require.NoError(t, err)
			owner, err := aphttp.New(context.Background(), directFetchLimits())
			require.NoError(t, err)
			t.Cleanup(owner.Close)
			requests := make(chan string, 2)
			input := directFetchInput(t, owner, httpHostClient{name: "captured", requests: requests}, nil)
			var buffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buffer, nil))
			result, err := program.Evaluate(context.Background(), func(runCtx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				host, err := jsruntime.NewHTTPHost(runCtx, vm, jsruntime.HTTPOptions{
					MaxRequestBytes: 8192, MaxPendingOperations: 1,
				}, limits)
				if err != nil {
					return err
				}
				// Cleanup closes admission synchronously before evidence is inspected.
				invocation.Close = func() { host.Close(); owner.Close() }
				invocation.Start, invocation.Await, invocation.Logger = host.Start, host.Await, logger
				return invocation.PrepareInput(runCtx, vm, input, limits, host)
			})

			// The failed message emits nothing, later effects stop, and the send remains known.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			records := loggingRecords(t, &buffer)
			require.Len(t, records, 1)
			require.Equal(t, "kept", records[0]["javascript"].(map[string]any)["message"])
			require.Len(t, requests, 1)
			require.Equal(t, "captured:/first", <-requests)
			require.Equal(t, aphttp.SendStateSent, owner.SendState())
			require.ErrorIs(t, owner.Err(), aphttp.ErrClosed)
		})
	}
}
