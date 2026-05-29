package app_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestResourceSnapshotTask_CreatesSamplesAndIsIdempotent(t *testing.T) {
	ctx, mainDB, store, retriever, handler := newResourceSnapshotTestHarness(t)
	now := time.Date(2026, time.May, 29, 12, 7, 0, 0, time.UTC)
	sampledAt := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)
	ctx = apctx.NewBuilder(ctx).WithClock(clock.NewFakeClock(now)).Build()

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, mainDB.CreateActor(ctx, &database.Actor{
		Id:         actorID,
		Namespace:  "root.metrics",
		ExternalId: "user-123",
		Labels:     database.Labels{"team": "platform"},
	}))

	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, mainDB.CreateConnection(ctx, &database.Connection{
		Id:               connID,
		Namespace:        "root.metrics",
		ConnectorId:      connectorID,
		ConnectorVersion: 2,
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateUnhealthy,
		Labels:           database.Labels{"connector": "crm"},
	}))

	require.NoError(t, handler.runResourceSnapshotTask(ctx, asynq.NewTask(taskTypeResourceSnapshot, nil)))
	require.NoError(t, handler.SnapshotResources(ctx, now))

	connectionSamples := listConnectionSamples(t, retriever, ctx, sampledAt)
	require.Len(t, connectionSamples, 1)
	require.Equal(t, connID, connectionSamples[0].ResourceID)
	require.Equal(t, sampledAt, connectionSamples[0].SampledAt)
	require.Equal(t, "root.metrics", connectionSamples[0].Namespace)
	require.Equal(t, database.ConnectionStateConfigured, connectionSamples[0].State)
	require.Equal(t, database.ConnectionHealthStateUnhealthy, connectionSamples[0].HealthState)
	require.Equal(t, connectorID, connectionSamples[0].ConnectorID)
	require.Equal(t, uint64(2), connectionSamples[0].ConnectorVersion)
	require.Equal(t, "crm", connectionSamples[0].Labels["connector"])

	actorSamples := listActorSamples(t, retriever, ctx, sampledAt)
	require.Len(t, actorSamples, 1)
	require.Equal(t, actorID, actorSamples[0].ResourceID)
	require.Equal(t, sampledAt, actorSamples[0].SampledAt)
	require.Equal(t, "root.metrics", actorSamples[0].Namespace)
	require.Equal(t, "user-123", actorSamples[0].ExternalID)
	require.Equal(t, "platform", actorSamples[0].Labels["team"])

	require.NoError(t, store.StoreActorResourceSamples(ctx, nil), "empty stores remain no-ops")
}

func TestResourceSnapshotTask_DeletedResourcesDoNotAppearInLaterSlices(t *testing.T) {
	ctx, mainDB, _, retriever, handler := newResourceSnapshotTestHarness(t)
	firstClock := clock.NewFakeClock(time.Date(2026, time.May, 29, 12, 7, 0, 0, time.UTC))
	ctx = apctx.NewBuilder(ctx).WithClock(firstClock).Build()
	firstSampledAt := time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC)
	secondSampledAt := time.Date(2026, time.May, 29, 12, 15, 0, 0, time.UTC)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, mainDB.CreateActor(ctx, &database.Actor{
		Id:         actorID,
		Namespace:  "root.metrics",
		ExternalId: "deleted-user",
	}))

	connID := apid.New(apid.PrefixConnection)
	require.NoError(t, mainDB.CreateConnection(ctx, &database.Connection{
		Id:               connID,
		Namespace:        "root.metrics",
		ConnectorId:      apid.New(apid.PrefixConnectorVersion),
		ConnectorVersion: 1,
		State:            database.ConnectionStateConfigured,
	}))

	require.NoError(t, handler.SnapshotResources(ctx, firstClock.Now()))

	firstClock.SetTime(time.Date(2026, time.May, 29, 12, 16, 0, 0, time.UTC))
	require.NoError(t, mainDB.DeleteActor(ctx, actorID))
	require.NoError(t, mainDB.DeleteConnection(ctx, connID))
	require.NoError(t, handler.SnapshotResources(ctx, firstClock.Now()))

	firstConnectionSamples := listConnectionSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstConnectionSamples, 1)
	secondConnectionSamples := listConnectionSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondConnectionSamples)

	firstActorSamples := listActorSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstActorSamples, 1)
	secondActorSamples := listActorSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondActorSamples)
}

func TestResourceSnapshotTask_CronUsesConfiguredInterval(t *testing.T) {
	handler := NewResourceSnapshotTaskHandler(
		nil,
		nil,
		&sconfig.AppMetrics{Database: &sconfig.Database{InnerVal: &sconfig.DatabaseSqlite{Path: "unused.db"}}},
		test_utils.NewTestLogger(),
	)

	tasks := handler.GetCronTasks()
	require.Len(t, tasks, 1)
	require.Equal(t, "@every 15m0s", tasks[0].Cronspec)
	require.Equal(t, taskTypeResourceSnapshot, tasks[0].Task.Type())

	handler.cfg.ResourceSnapshotInterval = &sconfig.HumanDuration{Duration: 5 * time.Minute}
	tasks = handler.GetCronTasks()
	require.Len(t, tasks, 1)
	require.Equal(t, "@every 5m0s", tasks[0].Cronspec)
}

func newResourceSnapshotTestHarness(t testing.TB) (
	context.Context,
	database.DB,
	ResourceSampleStore,
	ResourceSampleRetriever,
	*ResourceSnapshotTaskHandler,
) {
	t.Helper()

	_, mainDB := database.MustApplyBlankTestDbConfig(t, nil)
	require.NoError(t, mainDB.EnsureNamespaceByPath(context.Background(), "root.metrics"))
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	resourceRetriever := retriever.(ResourceSampleRetriever)
	handler := NewResourceSnapshotTaskHandler(
		mainDB,
		resourceStore,
		&sconfig.AppMetrics{
			Database:                 &sconfig.Database{InnerVal: &sconfig.DatabaseSqlite{Path: "unused.db"}},
			ResourceSnapshotInterval: &sconfig.HumanDuration{Duration: 15 * time.Minute},
		},
		test_utils.NewTestLogger(),
	)

	return context.Background(), mainDB, resourceStore, resourceRetriever, handler
}

func listConnectionSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*ConnectionResourceSample {
	t.Helper()
	samples, err := retriever.ListConnectionResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}

func listActorSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*ActorResourceSample {
	t.Helper()
	samples, err := retriever.ListActorResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}
