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
	connectorID := apid.New(apid.PrefixConnectorVersion)

	err := resourceStore.StoreConnectionResourceSamples(ctx, []*ConnectionResourceSample{
		{
			SampledAt:         start,
			ResourceID:        connID,
			Namespace:         "root.acme.prod",
			Labels:            database.Labels{"env": "prod", "team": "api"},
			State:             database.ConnectionStateConfigured,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  1,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
		{
			SampledAt:         start,
			ResourceID:        otherConnID,
			Namespace:         "root.acme.prod",
			Labels:            database.Labels{"env": "prod", "team": "api"},
			State:             database.ConnectionStateSetup,
			HealthState:       database.ConnectionHealthStateUnhealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  2,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
		{
			SampledAt:         start,
			ResourceID:        excludedConnID,
			Namespace:         "root.other",
			Labels:            database.Labels{"env": "prod", "team": "api"},
			State:             database.ConnectionStateConfigured,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  1,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
		{
			SampledAt:         start.Add(5 * time.Minute),
			ResourceID:        connID,
			Namespace:         "root.acme.prod",
			Labels:            database.Labels{"env": "prod", "team": "api"},
			State:             database.ConnectionStateConfigured,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  1,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start.Add(5 * time.Minute),
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
		GroupBy:           []ResourceGroupBy{ResourceGroupByState, ResourceGroupByHealthState, ResourceGroupByConnectorVersion},
	}})
	require.NoError(t, err)
	require.Len(t, series, 2)

	require.Equal(t, "connections", series[0].RefID)
	require.Equal(t, map[string]string{
		"connector_version": "1",
		"health_state":      string(database.ConnectionHealthStateHealthy),
		"state":             string(database.ConnectionStateConfigured),
	}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 0},
	}, series[0].Points)

	require.Equal(t, map[string]string{
		"connector_version": "2",
		"health_state":      string(database.ConnectionHealthStateUnhealthy),
		"state":             string(database.ConnectionStateSetup),
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

	require.Equal(t, map[string]string{"namespace": "root.acme"}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 0},
	}, series[0].Points)

	require.Equal(t, map[string]string{"namespace": "root.acme.ops"}, series[1].Labels)
	require.Equal(t, []ResourceMetricPoint{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(15 * time.Minute), Value: 0},
	}, series[1].Points)
}

func TestResourceMetrics_AdditionalResourceCounts(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)

	ctx := context.Background()
	start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	rateLimitID := apid.New(apid.PrefixRateLimit)

	require.NoError(t, resourceStore.StoreConnectorResourceSamples(ctx, []*ConnectorResourceSample{
		{
			SampledAt:         start,
			ResourceID:        connectorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod"},
			State:             database.ConnectorVersionStatePrimary,
			ConnectorVersion:  2,
			TotalVersions:     2,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
		},
	}))
	require.NoError(t, resourceStore.StoreConnectorVersionResourceSamples(ctx, []*ConnectorVersionResourceSample{
		{
			SampledAt:         start,
			ResourceID:        connectorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod"},
			State:             database.ConnectorVersionStatePrimary,
			ConnectorVersion:  2,
			ResourceCreatedAt: start.Add(-time.Hour),
			ResourceUpdatedAt: start,
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
			GroupBy:           []ResourceGroupBy{ResourceGroupByState, ResourceGroupByConnectorVersion},
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
			RefID:             "connector-versions",
			Metric:            ResourceMetricConnectorVersionsCount,
			Start:             start,
			End:               start.Add(30 * time.Minute),
			Step:              15 * time.Minute,
			NamespaceMatchers: []string{"root.acme"},
			LabelSelector:     "env=prod",
			GroupBy:           []ResourceGroupBy{ResourceGroupByConnectorID, ResourceGroupByConnectorVersion},
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
	require.Len(t, series, 4)

	require.Equal(t, "connectors", series[0].RefID)
	require.Equal(t, map[string]string{
		"connector_version": "2",
		"state":             string(database.ConnectorVersionStatePrimary),
	}, series[0].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 0}}, series[0].Points)

	require.Equal(t, "namespaces", series[1].RefID)
	require.Equal(t, map[string]string{"state": string(database.NamespaceStateActive)}, series[1].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 0}}, series[1].Points)

	require.Equal(t, "connector-versions", series[2].RefID)
	require.Equal(t, map[string]string{
		"connector_id":      connectorID.String(),
		"connector_version": "2",
	}, series[2].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 0}}, series[2].Points)

	require.Equal(t, "rate-limits", series[3].RefID)
	require.Equal(t, map[string]string{"mode": "observe"}, series[3].Labels)
	require.Equal(t, []ResourceMetricPoint{{Timestamp: start, Value: 1}, {Timestamp: start.Add(15 * time.Minute), Value: 0}}, series[3].Points)
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
