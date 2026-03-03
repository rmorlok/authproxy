package database

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestPurgeSoftDeletedRecords(t *testing.T) {
	now := time.Date(2024, time.January, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)

	setup := func(t *testing.T) (DB, context.Context, *clock.FakeClock) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

		// Create namespace
		require.NoError(t, db.EnsureNamespaceByPath(ctx, "root"))

		// Create an actor and soft-delete it (deleted 60 days ago)
		actorOld := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "old-deleted-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorOld))
		require.NoError(t, db.DeleteActor(ctx, actorOld.Id))
		// Backdate the deleted_at to 60 days ago
		_, err := rawDb.Exec("UPDATE actors SET deleted_at = ? WHERE id = ?",
			now.Add(-60*24*time.Hour).UTC().Format("2006-01-02 15:04:05"), actorOld.Id.String())
		require.NoError(t, err)

		// Create an actor and soft-delete it recently (5 days ago)
		actorRecent := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "recent-deleted-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorRecent))
		require.NoError(t, db.DeleteActor(ctx, actorRecent.Id))
		_, err = rawDb.Exec("UPDATE actors SET deleted_at = ? WHERE id = ?",
			now.Add(-5*24*time.Hour).UTC().Format("2006-01-02 15:04:05"), actorRecent.Id.String())
		require.NoError(t, err)

		// Create a live (non-deleted) actor
		actorLive := &Actor{
			Id:         apid.New(apid.PrefixActor),
			Namespace:  "root",
			ExternalId: "live-actor",
		}
		require.NoError(t, db.CreateActor(ctx, actorLive))

		// Verify starting state: 3 actors total, 2 soft-deleted
		require.Equal(t, 3, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM actors"))
		require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM actors WHERE deleted_at IS NOT NULL"))

		return db, ctx, clk
	}

	t.Run("deletes records older than threshold", func(t *testing.T) {
		db, ctx, _ := setup(t)

		// Purge records deleted more than 30 days ago
		olderThan := now.Add(-30 * 24 * time.Hour)
		deleted, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted) // only the 60-day-old record
	})

	t.Run("preserves records newer than threshold", func(t *testing.T) {
		db, ctx, _ := setup(t)
		_, rawDb := MustApplyBlankTestDbConfig(t, nil) // just to get rawDb reference
		_ = rawDb

		// Purge records deleted more than 30 days ago
		olderThan := now.Add(-30 * 24 * time.Hour)
		_, err := db.PurgeSoftDeletedRecords(ctx, olderThan)
		require.NoError(t, err)

		// The recently deleted actor (5 days) should still exist
		// The live actor should still exist
		// Only the 60-day-old actor should be gone
		// We can verify by trying another purge with a much more recent threshold
		olderThan2 := now.Add(-1 * 24 * time.Hour)
		deleted2, err := db.PurgeSoftDeletedRecords(ctx, olderThan2)
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted2) // the 5-day-old record
	})

	t.Run("preserves non-deleted records", func(t *testing.T) {
		db, ctx, _ := setup(t)

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

		deleted, err := db.PurgeSoftDeletedRecords(ctx, now)
		require.NoError(t, err)
		require.Equal(t, int64(0), deleted)
	})
}
