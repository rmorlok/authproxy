package app_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestResourceMetrics_ConnectionCountsUseStoredSamples(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)

	ctx := context.Background()
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	connID := apid.New(apid.PrefixConnection)
	otherConnID := apid.New(apid.PrefixConnection)
	excludedConnID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnector)

	// The first snapshot sees two matching connections and one outside the filters; by the second snapshot the
	// setup connection has been deleted.
	err := resourceStore.StoreConnectionResourceSamples(ctx, []*ConnectionResourceSample{
		{
			SampledAt:           start,
			ResourceID:          connID,
			Namespace:           "root.acme.prod",
			Labels:              database.Labels{"env": "prod", "team": "api"},
			State:               database.ConnectionStateConfigured,
			HealthState:         database.ConnectionHealthStateHealthy,
			ConnectorID:         connectorID,
			ConnectorGeneration: 1,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start,
		},
		{
			SampledAt:           start,
			ResourceID:          otherConnID,
			Namespace:           "root.acme.prod",
			Labels:              database.Labels{"env": "prod", "team": "api"},
			State:               database.ConnectionStateSetup,
			HealthState:         database.ConnectionHealthStateUnhealthy,
			ConnectorID:         connectorID,
			ConnectorGeneration: 2,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start,
		},
		{
			SampledAt:           start,
			ResourceID:          excludedConnID,
			Namespace:           "root.other",
			Labels:              database.Labels{"env": "prod", "team": "api"},
			State:               database.ConnectionStateConfigured,
			HealthState:         database.ConnectionHealthStateHealthy,
			ConnectorID:         connectorID,
			ConnectorGeneration: 1,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start,
		},
		{
			SampledAt:           start.Add(15 * time.Minute),
			ResourceID:          connID,
			Namespace:           "root.acme.prod",
			Labels:              database.Labels{"env": "prod", "team": "api"},
			State:               database.ConnectionStateConfigured,
			HealthState:         database.ConnectionHealthStateHealthy,
			ConnectorID:         connectorID,
			ConnectorGeneration: 1,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start.Add(15 * time.Minute),
		},
	})
	require.NoError(t, err)

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:             "connections",
		Metric:            ResourceMetricConnectionsCount,
		Start:             start,
		End:               start.Add(30 * time.Minute),
		Step:              15 * time.Minute,
		NamespaceMatchers: []string{"root.acme.**"},
		LabelSelector:     "env=prod",
		GroupBy:           []ResourceGroupBy{ResourceGroupByState, ResourceGroupByHealthState, ResourceGroupByConnectorGeneration},
	}})
	require.NoError(t, err)
	// Only connections matching the namespace matcher and label selector are counted.
	require.Len(t, series, 2)

	// The configured connection is present in both snapshots.
	require.Equal(t, "connections", series[0].RefID)
	require.Equal(t, map[string]string{
		"connector_generation": "1",
		"health_state":         string(database.ConnectionHealthStateHealthy),
		"state":                string(database.ConnectionStateConfigured),
	}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 1},
	}, series[0].Points)

	// The setup connection drops to zero in the snapshot taken after it was deleted.
	require.Equal(t, map[string]string{
		"connector_generation": "2",
		"health_state":         string(database.ConnectionHealthStateUnhealthy),
		"state":                string(database.ConnectionStateSetup),
	}, series[1].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 0},
	}, series[1].Points)
}

func TestResourceMetrics_ActorCountsByNamespace(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)

	ctx := context.Background()
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	actorID := apid.New(apid.PrefixActor)
	otherActorID := apid.New(apid.PrefixActor)
	excludedActorID := apid.New(apid.PrefixActor)

	err := resourceStore.StoreActorResourceSamples(ctx, []*ActorResourceSample{
		{
			SampledAt:         start,
			ResourceID:        actorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod"},
			ExternalID:        "actor-1",
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
		{
			SampledAt:         start,
			ResourceID:        otherActorID,
			Namespace:         "root.acme.ops",
			Labels:            database.Labels{"env": "prod"},
			ExternalID:        "actor-2",
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
		{
			SampledAt:         start,
			ResourceID:        excludedActorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "dev"},
			ExternalID:        "actor-3",
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
	})
	require.NoError(t, err)

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:             "actors",
		Metric:            ResourceMetricActorsCount,
		Start:             start,
		End:               start.Add(30 * time.Minute),
		Step:              15 * time.Minute,
		NamespaceMatchers: []string{"root.acme.**"},
		LabelSelector:     "env=prod",
		GroupBy:           []ResourceGroupBy{ResourceGroupByNamespace},
	}})
	require.NoError(t, err)
	require.Len(t, series, 2)

	// The single snapshot supplies the first bucket and is carried forward into the second, which has no snapshot of
	// its own.
	require.Equal(t, map[string]string{"namespace": "root.acme"}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 1},
	}, series[0].Points)

	require.Equal(t, map[string]string{"namespace": "root.acme.ops"}, series[1].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 1},
	}, series[1].Points)
}

func TestResourceMetrics_AdditionalResourceCounts(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)

	ctx := context.Background()
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	connectorID := apid.New(apid.PrefixConnector)
	rateLimitID := apid.New(apid.PrefixRateLimit)

	require.NoError(t, resourceStore.StoreConnectorResourceSamples(ctx, []*ConnectorResourceSample{
		{
			SampledAt:           start,
			ResourceID:          connectorID,
			Namespace:           "root.acme",
			Labels:              database.Labels{"env": "prod"},
			State:               database.ConnectorGenerationStatePrimary,
			ConnectorGeneration: 2,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start,
		},
	}))
	require.NoError(t, resourceStore.StoreConnectorGenerationResourceSamples(ctx, []*ConnectorGenerationResourceSample{
		{
			SampledAt:           start,
			ResourceID:          connectorID,
			Namespace:           "root.acme",
			Labels:              database.Labels{"env": "prod"},
			State:               database.ConnectorGenerationStatePrimary,
			ConnectorGeneration: 2,
			ResourceCreatedAt:   start.Add(-time.Hour),
			ResourceUpdatedAt:   start,
		},
	}))
	require.NoError(t, resourceStore.StoreNamespaceResourceSamples(ctx, []*NamespaceResourceSample{
		{
			SampledAt:         start,
			ResourceID:        "root.acme",
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod"},
			State:             database.NamespaceStateActive,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
	}))
	require.NoError(t, resourceStore.StoreRateLimitResourceSamples(ctx, []*RateLimitResourceSample{
		{
			SampledAt:         start,
			ResourceID:        rateLimitID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod"},
			Mode:              "observe",
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
	}))

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{
		{
			RefID:             "connectors",
			Metric:            ResourceMetricConnectorsCount,
			Start:             start,
			End:               start.Add(30 * time.Minute),
			Step:              15 * time.Minute,
			NamespaceMatchers: []string{"root.acme"},
			LabelSelector:     "env=prod",
			GroupBy:           []ResourceGroupBy{ResourceGroupByState, ResourceGroupByConnectorGeneration},
		},
		{
			RefID:             "namespaces",
			Metric:            ResourceMetricNamespacesCount,
			Start:             start,
			End:               start.Add(30 * time.Minute),
			Step:              15 * time.Minute,
			NamespaceMatchers: []string{"root.acme"},
			LabelSelector:     "env=prod",
			GroupBy:           []ResourceGroupBy{ResourceGroupByState},
		},
		{
			RefID:             "connector-generations",
			Metric:            ResourceMetricConnectorGenerationsCount,
			Start:             start,
			End:               start.Add(30 * time.Minute),
			Step:              15 * time.Minute,
			NamespaceMatchers: []string{"root.acme"},
			LabelSelector:     "env=prod",
			GroupBy:           []ResourceGroupBy{ResourceGroupByConnectorID, ResourceGroupByConnectorGeneration},
		},
		{
			RefID:             "rate-limits",
			Metric:            ResourceMetricRateLimitsCount,
			Start:             start,
			End:               start.Add(30 * time.Minute),
			Step:              15 * time.Minute,
			NamespaceMatchers: []string{"root.acme"},
			LabelSelector:     "env=prod",
			GroupBy:           []ResourceGroupBy{ResourceGroupByMode},
		},
	})
	require.NoError(t, err)
	// Every metric counts its one resource in both buckets, the second carried forward from the only snapshot.
	require.Len(t, series, 4)

	require.Equal(t, "connectors", series[0].RefID)
	require.Equal(t, map[string]string{
		"connector_generation": "2",
		"state":                string(database.ConnectorGenerationStatePrimary),
	}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 1}}, series[0].Points)

	require.Equal(t, "namespaces", series[1].RefID)
	require.Equal(t, map[string]string{"state": string(database.NamespaceStateActive)}, series[1].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 1}}, series[1].Points)

	require.Equal(t, "connector-generations", series[2].RefID)
	require.Equal(t, map[string]string{
		"connector_id":         connectorID.String(),
		"connector_generation": "2",
	}, series[2].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 1}}, series[2].Points)

	require.Equal(t, "rate-limits", series[3].RefID)
	require.Equal(t, map[string]string{"mode": "observe"}, series[3].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 1}}, series[3].Points)
}

func TestResourceMetrics_InvalidGroupBy(t *testing.T) {
	_, retriever, _ := MustNewBlankRequestEventsStore(t)
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

	_, err := retriever.QueryResourceMetrics(context.Background(), []ResourceMetricsQuery{{
		RefID:   "actors",
		Metric:  ResourceMetricActorsCount,
		Start:   start,
		End:     start.Add(15 * time.Minute),
		Step:    15 * time.Minute,
		GroupBy: []ResourceGroupBy{ResourceGroupByState},
	}})
	require.Error(t, err)
}

func TestResourceMetrics_LatestSnapshotInBucketWins(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)

	ctx := context.Background()
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnector)
	sample := func(at time.Time, state database.ConnectionState) *ConnectionResourceSample {
		return &ConnectionResourceSample{
			SampledAt:         at,
			ResourceID:        connID,
			Namespace:         "root",
			State:             state,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ResourceCreatedAt: start,
			ResourceUpdatedAt: at,
		}
	}

	// One connection is sampled in setup, then configured, inside the same 15m bucket.
	require.NoError(t, resourceStore.StoreConnectionResourceSamples(ctx, []*ConnectionResourceSample{
		sample(start, database.ConnectionStateSetup),
		sample(start.Add(5*time.Minute), database.ConnectionStateConfigured),
	}))

	query := func(groupBy ...ResourceGroupBy) ResourceMetricsQuery {
		return ResourceMetricsQuery{
			RefID:   "connections",
			Metric:  ResourceMetricConnectionsCount,
			Start:   start,
			End:     start.Add(15 * time.Minute),
			Step:    15 * time.Minute,
			GroupBy: groupBy,
		}
	}

	// Grouped by state, the bucket reports the later snapshot, so the connection is counted only as configured.
	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{query(ResourceGroupByState)})
	require.NoError(t, err)
	byState := map[string]float64{}
	for _, s := range series {
		byState[s.Labels["state"]] = s.Points[0].Value
	}
	require.Equal(t, map[string]float64{
		string(database.ConnectionStateConfigured): 1,
	}, byState)

	// Ungrouped, it is a single resource.
	series, err = retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{query()})
	require.NoError(t, err)
	require.Len(t, series, 1)
	require.Equal(t, 1.0, series[0].Points[0].Value)
}

// storeConnectionSnapshot writes one snapshot of the given connections, all in the configured state unless the
// resource ID appears in setupIDs.
func storeConnectionSnapshot(
	t *testing.T,
	store ResourceSampleStore,
	at time.Time,
	namespace string,
	ids []apid.ID,
	setupIDs ...apid.ID,
) {
	t.Helper()
	setup := map[apid.ID]bool{}
	for _, id := range setupIDs {
		setup[id] = true
	}
	samples := make([]*ConnectionResourceSample, 0, len(ids))
	for _, id := range ids {
		state := database.ConnectionStateConfigured
		if setup[id] {
			state = database.ConnectionStateSetup
		}
		samples = append(samples, &ConnectionResourceSample{
			SampledAt:         at,
			ResourceID:        id,
			Namespace:         namespace,
			Labels:            database.Labels{"env": "prod"},
			State:             state,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       apid.New(apid.PrefixConnector),
			ResourceCreatedAt: at.Add(-time.Hour),
			ResourceUpdatedAt: at,
		})
	}
	require.NoError(t, store.StoreConnectionResourceSamples(context.Background(), samples))
}

func TestResourceMetrics_CarriesSnapshotsIntoMisalignedAndShortBuckets(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	ctx := context.Background()

	// Snapshots land on 15m clock boundaries with two, three, then four connections.
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	a, b := apid.New(apid.PrefixConnection), apid.New(apid.PrefixConnection)
	c, d := apid.New(apid.PrefixConnection), apid.New(apid.PrefixConnection)
	storeConnectionSnapshot(t, resourceStore, base, "root", []apid.ID{a, b})
	storeConnectionSnapshot(t, resourceStore, base.Add(15*time.Minute), "root", []apid.ID{a, b, c})
	storeConnectionSnapshot(t, resourceStore, base.Add(30*time.Minute), "root", []apid.ID{a, b, c, d})

	// A 15m step starting 7 minutes off the snapshot grid leaves some buckets with no snapshot inside them.
	start := base.Add(7 * time.Minute)
	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:          "connections",
		Metric:         ResourceMetricConnectionsCount,
		Start:          start,
		End:            start.Add(45 * time.Minute),
		Step:           15 * time.Minute,
		SampleInterval: 15 * time.Minute,
	}})
	require.NoError(t, err)
	require.Len(t, series, 1)

	// Every bucket reports the latest snapshot before its end; the last bucket has no snapshot of its own but carries
	// 12:30 forward instead of dropping to zero.
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 3},                       // [12:07, 12:22) -> 12:15 snapshot
		{Timestamp: start.Add(15 * time.Minute), Value: 4}, // [12:22, 12:37) -> 12:30 snapshot
		{Timestamp: start.Add(30 * time.Minute), Value: 4}, // [12:37, 12:52) -> 12:30 carried forward
	}, series[0].Points)

	// A 1m step, much shorter than the snapshot interval, carries each snapshot across the minutes that follow it.
	series, err = retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:          "connections",
		Metric:         ResourceMetricConnectionsCount,
		Start:          base.Add(13 * time.Minute),
		End:            base.Add(17 * time.Minute),
		Step:           time.Minute,
		SampleInterval: 15 * time.Minute,
	}})
	require.NoError(t, err)
	require.Len(t, series, 1)
	values := make([]float64, 0, len(series[0].Points))
	for _, p := range series[0].Points {
		values = append(values, p.Value)
	}
	// 12:13 and 12:14 still read the 12:00 snapshot (taken before the range); from 12:15 the new snapshot applies.
	require.Equal(t, []float64{2, 2, 3, 3}, values)
}

func TestResourceMetrics_StaleSnapshotsBecomeGaps(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	ctx := context.Background()

	// A single snapshot, after which the snapshot task stops running.
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	storeConnectionSnapshot(t, resourceStore, base, "root", []apid.ID{apid.New(apid.PrefixConnection)})

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:          "connections",
		Metric:         ResourceMetricConnectionsCount,
		Start:          base.Add(-15 * time.Minute),
		End:            base.Add(75 * time.Minute),
		Step:           15 * time.Minute,
		SampleInterval: 15 * time.Minute,
	}})
	require.NoError(t, err)
	require.Len(t, series, 1)

	// The bucket before the first snapshot has no point, the snapshot carries forward for up to 2.5 intervals
	// (37.5m) past each bucket's end, and later buckets are gaps rather than zeros.
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: base, Value: 1},                       // [12:00, 12:15)
		{Timestamp: base.Add(15 * time.Minute), Value: 1}, // [12:15, 12:30): snapshot is 30m old at bucket end
	}, series[0].Points)
}

func TestResourceMetrics_FilteredSnapshotWithNoMatchesIsZero(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	ctx := context.Background()

	// The first snapshot has a connection in root.acme; by the second it is gone, but another namespace still has one.
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	storeConnectionSnapshot(t, resourceStore, base, "root.acme", []apid.ID{apid.New(apid.PrefixConnection)})
	storeConnectionSnapshot(t, resourceStore, base.Add(15*time.Minute), "root.other", []apid.ID{apid.New(apid.PrefixConnection)})

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:             "connections",
		Metric:            ResourceMetricConnectionsCount,
		Start:             base,
		End:               base.Add(30 * time.Minute),
		Step:              15 * time.Minute,
		SampleInterval:    15 * time.Minute,
		NamespaceMatchers: []string{"root.acme"},
	}})
	require.NoError(t, err)
	require.Len(t, series, 1)

	// The second snapshot matches nothing in root.acme, so it reads as zero instead of carrying the first forward.
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: base, Value: 1},
		{Timestamp: base.Add(15 * time.Minute), Value: 0},
	}, series[0].Points)
}

func TestResourceMetrics_GroupsMissingFromSnapshotReadZero(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	ctx := context.Background()

	// A connection is in setup at the first snapshot and configured at the second.
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	id := apid.New(apid.PrefixConnection)
	storeConnectionSnapshot(t, resourceStore, base, "root", []apid.ID{id}, id)
	storeConnectionSnapshot(t, resourceStore, base.Add(15*time.Minute), "root", []apid.ID{id})

	series, err := retriever.QueryResourceMetrics(ctx, []ResourceMetricsQuery{{
		RefID:          "connections",
		Metric:         ResourceMetricConnectionsCount,
		Start:          base,
		End:            base.Add(30 * time.Minute),
		Step:           15 * time.Minute,
		SampleInterval: 15 * time.Minute,
		GroupBy:        []ResourceGroupBy{ResourceGroupByState},
	}})
	require.NoError(t, err)
	require.Len(t, series, 2)

	// Both state series have a point in every bucket that has a snapshot, zero where the state was not observed.
	byState := map[string][]float64{}
	for _, s := range series {
		for _, p := range s.Points {
			byState[s.Labels["state"]] = append(byState[s.Labels["state"]], p.Value)
		}
	}
	require.Equal(t, map[string][]float64{
		string(database.ConnectionStateSetup):      {1, 0},
		string(database.ConnectionStateConfigured): {0, 1},
	}, byState)
}

func TestSelectResourceMetricBucketSnapshots(t *testing.T) {
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	query := ResourceMetricsQuery{
		Start:          base,
		End:            base.Add(50 * time.Minute),
		Step:           10 * time.Minute,
		SampleInterval: 10 * time.Minute,
	}

	// Snapshots at 11:55 (before the range), 12:00, and 12:05.
	got := selectResourceMetricBucketSnapshots(query, []int64{
		ms(base.Add(-5 * time.Minute)),
		ms(base),
		ms(base.Add(5 * time.Minute)),
	})

	// The first bucket takes its latest snapshot (12:05), which then carries forward while it is within 25m of
	// each bucket's end; the last bucket (ending 12:50, 45m later) has no usable snapshot.
	require.Equal(t, []int64{
		ms(base.Add(5 * time.Minute)),
		ms(base.Add(5 * time.Minute)),
		ms(base.Add(5 * time.Minute)),
		-1,
		-1,
	}, got)

	// With no snapshots at all every bucket is empty.
	require.Equal(t, []int64{-1, -1, -1, -1, -1}, selectResourceMetricBucketSnapshots(query, nil))
}
