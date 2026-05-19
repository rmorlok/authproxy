package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestProbeOutcome_InsertAppendsRow(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	row, err := db.InsertProbeOutcome(ctx, connectionId, "ping", ProbeOutcomeStatusFailure, "connection refused")
	require.NoError(t, err)
	require.True(t, row.Id.HasPrefix(apid.PrefixProbeOutcome))
	require.Equal(t, connectionId, row.ConnectionId)
	require.Equal(t, "ping", row.ProbeId)
	require.Equal(t, ProbeOutcomeStatusFailure, row.Outcome)
	require.NotNil(t, row.ErrorMessage)
	require.Equal(t, "connection refused", *row.ErrorMessage)
	require.True(t, now.Equal(row.OccurredAt))
}

func TestProbeOutcome_SuccessOmitsErrorMessage(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	row, err := db.InsertProbeOutcome(ctx, apid.New(apid.PrefixConnection), "ping", ProbeOutcomeStatusSuccess, "")
	require.NoError(t, err)
	require.Nil(t, row.ErrorMessage)
}

func TestProbeOutcome_GetRecent_OrderedNewestFirst(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	// Three outcomes at t=0, t=1m, t=2m.
	for i, o := range []string{ProbeOutcomeStatusSuccess, ProbeOutcomeStatusFailure, ProbeOutcomeStatusFailure} {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Minute))).Build()
		_, err := db.InsertProbeOutcome(ctx, connectionId, "ping", o, "")
		require.NoError(t, err)
	}

	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	rows, err := db.GetRecentProbeOutcomes(ctx, connectionId, "ping", 10)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	// Newest first.
	require.Equal(t, ProbeOutcomeStatusFailure, rows[0].Outcome)
	require.Equal(t, ProbeOutcomeStatusFailure, rows[1].Outcome)
	require.Equal(t, ProbeOutcomeStatusSuccess, rows[2].Outcome)
}

func TestProbeOutcome_GetRecent_RespectsLimit(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Minute))).Build()
		_, err := db.InsertProbeOutcome(ctx, connectionId, "ping", ProbeOutcomeStatusFailure, "")
		require.NoError(t, err)
	}

	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	rows, err := db.GetRecentProbeOutcomes(ctx, connectionId, "ping", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestProbeOutcome_GetRecent_DistinctScopes(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connA := apid.New(apid.PrefixConnection)
	connB := apid.New(apid.PrefixConnection)
	_, _ = db.InsertProbeOutcome(ctx, connA, "ping", ProbeOutcomeStatusFailure, "")
	_, _ = db.InsertProbeOutcome(ctx, connA, "pong", ProbeOutcomeStatusSuccess, "")
	_, _ = db.InsertProbeOutcome(ctx, connB, "ping", ProbeOutcomeStatusSuccess, "")

	rows, err := db.GetRecentProbeOutcomes(ctx, connA, "ping", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, ProbeOutcomeStatusFailure, rows[0].Outcome)
}

func TestProbeOutcome_DeleteOld_RespectsKeepMinimum(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	// 5 outcomes at t = 0..4 hours.
	for i := 0; i < 5; i++ {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Hour))).Build()
		_, err := db.InsertProbeOutcome(ctx, connectionId, "ping", ProbeOutcomeStatusFailure, "")
		require.NoError(t, err)
	}

	// Cutoff at base+3h; without keep-minimum, rows at t=0,1,2 would go.
	// With keep-minimum=3, the most recent 3 (t=2,3,4) are protected; only
	// t=0 and t=1 are deletable.
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	deleted, err := db.DeleteOldProbeOutcomes(ctx, connectionId, "ping", 3, base.Add(3*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)

	n, err := db.CountProbeOutcomes(ctx, connectionId, "ping")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestProbeOutcome_DeleteOld_KeepMinExceedsTotal(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	// Only 2 outcomes total — all should be protected by keep-minimum=5.
	for i := 0; i < 2; i++ {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Hour))).Build()
		_, err := db.InsertProbeOutcome(ctx, connectionId, "ping", ProbeOutcomeStatusFailure, "")
		require.NoError(t, err)
	}

	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	deleted, err := db.DeleteOldProbeOutcomes(ctx, connectionId, "ping", 5, base.Add(10*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted, "keep-minimum > total: nothing should be deleted")

	n, err := db.CountProbeOutcomes(ctx, connectionId, "ping")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestProbeOutcome_DeleteOld_NoProtection(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Hour))).Build()
		_, err := db.InsertProbeOutcome(ctx, connectionId, "ping", ProbeOutcomeStatusFailure, "")
		require.NoError(t, err)
	}

	// keepMinimum=0 means anything older than cutoff is deletable.
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	deleted, err := db.DeleteOldProbeOutcomes(ctx, connectionId, "ping", 0, base.Add(2*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)

	n, err := db.CountProbeOutcomes(ctx, connectionId, "ping")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestProbeOutcome_DistinctProbeIds(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	_, _ = db.InsertProbeOutcome(ctx, connectionId, "alpha", ProbeOutcomeStatusFailure, "")
	_, _ = db.InsertProbeOutcome(ctx, connectionId, "alpha", ProbeOutcomeStatusSuccess, "")
	_, _ = db.InsertProbeOutcome(ctx, connectionId, "beta", ProbeOutcomeStatusFailure, "")

	ids, err := db.DistinctProbeIdsForConnection(ctx, connectionId)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", "beta"}, ids)
}

func TestProbeOutcome_DeleteOld_OnlyAffectsTargetProbe(t *testing.T) {
	// Verifies the cleanup query is scoped to (connection, probe) — pruning
	// outcomes for one probe must not touch a sibling probe's rows.
	_, db := MustApplyBlankTestDbConfig(t, nil)
	connectionId := apid.New(apid.PrefixConnection)
	base := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(base.Add(time.Duration(i) * time.Hour))).Build()
		_, _ = db.InsertProbeOutcome(ctx, connectionId, "alpha", ProbeOutcomeStatusFailure, "")
		_, _ = db.InsertProbeOutcome(ctx, connectionId, "beta", ProbeOutcomeStatusFailure, "")
	}

	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()
	deleted, err := db.DeleteOldProbeOutcomes(ctx, connectionId, "alpha", 0, base.Add(10*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 3, deleted)

	betaCount, err := db.CountProbeOutcomes(ctx, connectionId, "beta")
	require.NoError(t, err)
	assert.Equal(t, 3, betaCount, "sibling probe must be untouched")
}
