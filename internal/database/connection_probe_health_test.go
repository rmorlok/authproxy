package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestConnectionProbeHealth_FirstFailureCreatesRow(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	row, err := db.RecordProbeFailure(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 1, row.ConsecutiveFailures)
	require.Equal(t, 0, row.ConsecutiveSuccesses)
	require.NotNil(t, row.LastOutcome)
	require.Equal(t, ProbeOutcomeStatusFailure, *row.LastOutcome)
}

func TestConnectionProbeHealth_FirstSuccessCreatesRow(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	row, err := db.RecordProbeSuccess(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 0, row.ConsecutiveFailures)
	require.Equal(t, 1, row.ConsecutiveSuccesses)
}

func TestConnectionProbeHealth_FailuresIncrementAndSuccessResets(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)

	row, err := db.RecordProbeFailure(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 1, row.ConsecutiveFailures)

	row, err = db.RecordProbeFailure(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 2, row.ConsecutiveFailures)

	row, err = db.RecordProbeFailure(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 3, row.ConsecutiveFailures)

	// A success after failures resets the failure counter and starts the
	// success streak at 1.
	row, err = db.RecordProbeSuccess(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 0, row.ConsecutiveFailures, "success must reset failure streak")
	require.Equal(t, 1, row.ConsecutiveSuccesses)
}

func TestConnectionProbeHealth_SuccessesIncrementAndFailureResets(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	_, err := db.RecordProbeSuccess(ctx, connectionId, "ping")
	require.NoError(t, err)
	row, err := db.RecordProbeSuccess(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 2, row.ConsecutiveSuccesses)

	row, err = db.RecordProbeFailure(ctx, connectionId, "ping")
	require.NoError(t, err)
	require.Equal(t, 0, row.ConsecutiveSuccesses, "failure must reset success streak")
	require.Equal(t, 1, row.ConsecutiveFailures)
}

func TestConnectionProbeHealth_DistinctRowsPerProbe(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	_, err := db.RecordProbeFailure(ctx, connectionId, "ping-a")
	require.NoError(t, err)
	_, err = db.RecordProbeFailure(ctx, connectionId, "ping-a")
	require.NoError(t, err)
	_, err = db.RecordProbeSuccess(ctx, connectionId, "ping-b")
	require.NoError(t, err)

	all, err := db.ListConnectionProbeHealth(ctx, connectionId)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, 2, all["ping-a"].ConsecutiveFailures)
	require.Equal(t, 1, all["ping-b"].ConsecutiveSuccesses)
}

func TestConnectionProbeHealth_DistinctRowsPerConnection(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionA := apid.New(apid.PrefixConnection)
	connectionB := apid.New(apid.PrefixConnection)

	_, err := db.RecordProbeFailure(ctx, connectionA, "ping")
	require.NoError(t, err)
	_, err = db.RecordProbeFailure(ctx, connectionA, "ping")
	require.NoError(t, err)
	_, err = db.RecordProbeFailure(ctx, connectionB, "ping")
	require.NoError(t, err)

	a, err := db.GetConnectionProbeHealth(ctx, connectionA, "ping")
	require.NoError(t, err)
	require.Equal(t, 2, a.ConsecutiveFailures)

	b, err := db.GetConnectionProbeHealth(ctx, connectionB, "ping")
	require.NoError(t, err)
	require.Equal(t, 1, b.ConsecutiveFailures)
}

func TestConnectionProbeHealth_GetReturnsNotFound(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	_, err := db.GetConnectionProbeHealth(ctx, apid.New(apid.PrefixConnection), "ping")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestConnectionProbeHealth_ResetZerosCounters(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	_, err := db.RecordProbeFailure(ctx, connectionId, "a")
	require.NoError(t, err)
	_, err = db.RecordProbeFailure(ctx, connectionId, "a")
	require.NoError(t, err)
	_, err = db.RecordProbeSuccess(ctx, connectionId, "b")
	require.NoError(t, err)

	require.NoError(t, db.ResetConnectionProbeHealth(ctx, connectionId))

	all, err := db.ListConnectionProbeHealth(ctx, connectionId)
	require.NoError(t, err)
	require.Len(t, all, 2)
	for _, row := range all {
		require.Equal(t, 0, row.ConsecutiveFailures)
		require.Equal(t, 0, row.ConsecutiveSuccesses)
	}
}

func TestConnectionProbeHealth_ResetOnUnknownConnectionIsNoop(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	// No rows exist. Reset should succeed without error.
	require.NoError(t, db.ResetConnectionProbeHealth(ctx, apid.New(apid.PrefixConnection)))
}
