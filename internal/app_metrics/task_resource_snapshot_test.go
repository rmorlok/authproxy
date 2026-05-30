package app_metrics

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestResourceSnapshotTask_CreatesSamplesAndIsIdempotent(t *testing.T) {
	ctx, mainDB, rawDB, store, retriever, handler := newResourceSnapshotTestHarness(t)
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

	connectorResourceID := apid.New(apid.PrefixConnectorVersion)
	insertConnectorVersion(t, rawDB, connectorResourceID, "root.metrics", 1, database.ConnectorVersionStatePrimary)

	rateLimitID := apid.New(apid.PrefixRateLimit)
	require.NoError(t, mainDB.CreateRateLimit(ctx, &database.RateLimit{
		Id:         rateLimitID,
		Namespace:  "root.metrics",
		Definition: validSnapshotRateLimitDef(),
		Labels:     database.Labels{"scope": "proxy"},
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

	connectorSamples := listConnectorSamples(t, retriever, ctx, sampledAt)
	require.Len(t, connectorSamples, 1)
	require.Equal(t, connectorResourceID, connectorSamples[0].ResourceID)
	require.Equal(t, sampledAt, connectorSamples[0].SampledAt)
	require.Equal(t, "root.metrics", connectorSamples[0].Namespace)
	require.Equal(t, database.ConnectorVersionStatePrimary, connectorSamples[0].State)
	require.Equal(t, uint64(1), connectorSamples[0].ConnectorVersion)
	require.Equal(t, int64(1), connectorSamples[0].TotalVersions)

	connectorVersionSamples := listConnectorVersionSamples(t, retriever, ctx, sampledAt)
	require.Len(t, connectorVersionSamples, 1)
	require.Equal(t, connectorResourceID, connectorVersionSamples[0].ResourceID)
	require.Equal(t, uint64(1), connectorVersionSamples[0].ConnectorVersion)
	require.Equal(t, database.ConnectorVersionStatePrimary, connectorVersionSamples[0].State)

	namespaceSamples := listNamespaceSamples(t, retriever, ctx, sampledAt)
	require.Contains(t, namespaceSampleIDs(namespaceSamples), "root.metrics")

	rateLimitSamples := listRateLimitSamples(t, retriever, ctx, sampledAt)
	require.Len(t, rateLimitSamples, 1)
	require.Equal(t, rateLimitID, rateLimitSamples[0].ResourceID)
	require.Equal(t, "root.metrics", rateLimitSamples[0].Namespace)
	require.Equal(t, string(rlschema.ModeObserve), rateLimitSamples[0].Mode)
	require.Equal(t, "proxy", rateLimitSamples[0].Labels["scope"])

	require.NoError(t, store.StoreActorResourceSamples(ctx, nil), "empty stores remain no-ops")
}

func TestResourceSnapshotTask_DeletedResourcesDoNotAppearInLaterSlices(t *testing.T) {
	ctx, mainDB, rawDB, _, retriever, handler := newResourceSnapshotTestHarness(t)
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

	connectorResourceID := apid.New(apid.PrefixConnectorVersion)
	insertConnectorVersion(t, rawDB, connectorResourceID, "root.metrics", 1, database.ConnectorVersionStatePrimary)

	require.NoError(t, mainDB.EnsureNamespaceByPath(ctx, "root.metrics.deleted"))

	rateLimitID := apid.New(apid.PrefixRateLimit)
	require.NoError(t, mainDB.CreateRateLimit(ctx, &database.RateLimit{
		Id:         rateLimitID,
		Namespace:  "root.metrics",
		Definition: validSnapshotRateLimitDef(),
	}))

	require.NoError(t, handler.SnapshotResources(ctx, firstClock.Now()))

	firstClock.SetTime(time.Date(2026, time.May, 29, 12, 16, 0, 0, time.UTC))
	require.NoError(t, mainDB.DeleteActor(ctx, actorID))
	require.NoError(t, mainDB.DeleteConnection(ctx, connID))
	require.NoError(t, mainDB.DeleteConnector(ctx, connectorResourceID))
	require.NoError(t, mainDB.DeleteNamespace(ctx, "root.metrics.deleted"))
	require.NoError(t, mainDB.DeleteRateLimit(ctx, rateLimitID))
	require.NoError(t, handler.SnapshotResources(ctx, firstClock.Now()))

	firstConnectionSamples := listConnectionSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstConnectionSamples, 1)
	secondConnectionSamples := listConnectionSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondConnectionSamples)

	firstActorSamples := listActorSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstActorSamples, 1)
	secondActorSamples := listActorSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondActorSamples)

	firstConnectorSamples := listConnectorSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstConnectorSamples, 1)
	secondConnectorSamples := listConnectorSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondConnectorSamples)

	firstConnectorVersionSamples := listConnectorVersionSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstConnectorVersionSamples, 1)
	secondConnectorVersionSamples := listConnectorVersionSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondConnectorVersionSamples)

	firstNamespaceSamples := listNamespaceSamples(t, retriever, ctx, firstSampledAt)
	require.Contains(t, namespaceSampleIDs(firstNamespaceSamples), "root.metrics.deleted")
	secondNamespaceSamples := listNamespaceSamples(t, retriever, ctx, secondSampledAt)
	require.NotContains(t, namespaceSampleIDs(secondNamespaceSamples), "root.metrics.deleted")

	firstRateLimitSamples := listRateLimitSamples(t, retriever, ctx, firstSampledAt)
	require.Len(t, firstRateLimitSamples, 1)
	secondRateLimitSamples := listRateLimitSamples(t, retriever, ctx, secondSampledAt)
	require.Empty(t, secondRateLimitSamples)
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
	*sql.DB,
	ResourceSampleStore,
	ResourceSampleRetriever,
	*ResourceSnapshotTaskHandler,
) {
	t.Helper()

	_, mainDB, rawDB := database.MustApplyBlankTestDbConfigRaw(t, nil)
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

	return context.Background(), mainDB, rawDB, resourceStore, resourceRetriever, handler
}

func insertConnectorVersion(
	t testing.TB,
	rawDB *sql.DB,
	id apid.ID,
	namespace string,
	version uint64,
	state database.ConnectorVersionState,
) {
	t.Helper()
	_, err := rawDB.Exec(fmt.Sprintf(`
INSERT INTO connector_versions
(id, namespace, version, state, type, encrypted_definition, hash, created_at, updated_at, deleted_at)
VALUES
('%s', '%s', %d, '%s', 'test', '{"id":"ekv_test","d":"encrypted-def"}', 'hash-%d', '2026-05-29 12:00:00', '2026-05-29 12:00:00', null)
`, id, namespace, version, state, version))
	require.NoError(t, err)
}

func validSnapshotRateLimitDef() rlschema.RateLimit {
	return rlschema.RateLimit{
		Mode: rlschema.ModeObserve,
		Selector: rlschema.Selector{
			Methods:      []string{"GET"},
			RequestTypes: []common.RequestType{common.RequestTypeProxy},
		},
		Bucket: rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 60, RefillRate: 1},
		},
	}
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

func listConnectorSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*ConnectorResourceSample {
	t.Helper()
	samples, err := retriever.ListConnectorResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}

func listConnectorVersionSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*ConnectorVersionResourceSample {
	t.Helper()
	samples, err := retriever.ListConnectorVersionResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}

func listNamespaceSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*NamespaceResourceSample {
	t.Helper()
	samples, err := retriever.ListNamespaceResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}

func namespaceSampleIDs(samples []*NamespaceResourceSample) []string {
	ids := make([]string, 0, len(samples))
	for _, sample := range samples {
		ids = append(ids, sample.ResourceID)
	}
	return ids
}

func listRateLimitSamples(
	t testing.TB,
	retriever ResourceSampleRetriever,
	ctx context.Context,
	sampledAt time.Time,
) []*RateLimitResourceSample {
	t.Helper()
	samples, err := retriever.ListRateLimitResourceSamples(ctx, ResourceSampleQuery{
		Start: &sampledAt,
		End:   &sampledAt,
	})
	require.NoError(t, err)
	return samples
}
