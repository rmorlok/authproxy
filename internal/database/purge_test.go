package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestPurgeSoftDeletedRecords(t *testing.T) {
	// Start the clock 60 days in the past
	start := time.Date(2024, time.January, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(start)

	setup := func(t *testing.T) (DB, *clock.FakeClock) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		// Reset clock to start
		*clk = *clock.NewFakeClock(start)

		// Create namespace
		require.NoError(t, db.EnsureNamespaceByPath(ctx, "root"))

		// Create and soft-delete an actor at time "start" (will be 60 days old)
		actorOld := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "old-deleted-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorOld))
		require.NoError(t, db.DeleteActor(ctx, actorOld.Id))

		// Advance clock 55 days, then create and delete another actor (will be 5 days old at "now")
		clk.Step(55 * 24 * time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()

		actorRecent := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "recent-deleted-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorRecent))
		require.NoError(t, db.DeleteActor(ctx, actorRecent.Id))

		// Advance clock 5 more days to "now" (60 days from start), create a live actor
		clk.Step(5 * 24 * time.Hour)
		ctx = apctx.NewBuilderBackground().WithClock(clk).Build()

		actorLive := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "live-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorLive))

		// Verify starting state: 3 actors total, 2 soft-deleted
		require.Equal(t, 3, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM actors"))
		require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM actors WHERE deleted_at IS NOT NULL"))

		return db, clk
	}

	t.Run("deletes records older than threshold", func(t *testing.T) {
		db, clk := setup(t)
		now := clk.Now()
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		// Purge records deleted more than 30 days ago
		olderThan := now.Add(-30 * 24 * time.Hour)
		deleted, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted) // only the 60-day-old record
	})

	t.Run("preserves records newer than threshold", func(t *testing.T) {
		db, clk := setup(t)
		now := clk.Now()
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		// Purge records deleted more than 30 days ago
		olderThan := now.Add(-30 * 24 * time.Hour)
		_, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)

		// The recently deleted actor (5 days) should still exist
		// We can verify by trying another purge with a much more recent threshold
		olderThan2 := now.Add(-1 * 24 * time.Hour)
		deleted2, err := db.PurgeSoftDeletedRecords(ctx, olderThan2)
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted2) // the 5-day-old record
	})

	t.Run("preserves non-deleted records", func(t *testing.T) {
		db, clk := setup(t)
		now := clk.Now()
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		// Purge everything older than 1 day
		olderThan := now.Add(-1 * 24 * time.Hour)
		deleted, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)
		require.Equal(t, int64(2), deleted) // both deleted actors

		// A third purge should find nothing
		deleted2, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)
		require.Equal(t, int64(0), deleted2)
	})

	t.Run("returns zero when no records to purge", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		deleted, err := db.PurgeSoftDeletedRecords(ctx, clk.Now())
		require.NoError(t, err)
		require.Equal(t, int64(0), deleted)
	})
}
