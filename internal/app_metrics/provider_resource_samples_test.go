package app_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestResourceSamples_ConnectionSamples_IdempotentAndQueryable(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	resourceRetriever := retriever.(ResourceSampleRetriever)

	ctx := context.Background()
	sampledAt := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	connID := apid.New(apid.PrefixConnection)
	otherConnID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	createdAt := sampledAt.Add(-time.Hour)
	updatedAt := sampledAt.Add(-time.Minute)

	err := resourceStore.StoreConnectionResourceSamples(ctx, []*ConnectionResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        connID,
			Namespace:         "root.acme.prod",
			Labels:            database.Labels{"env": "prod", "tier": "silver"},
			State:             database.ConnectionStateSetup,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  1,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
		{
			SampledAt:         sampledAt,
			ResourceID:        otherConnID,
			Namespace:         "root.other",
			Labels:            database.Labels{"env": "prod", "tier": "gold"},
			State:             database.ConnectionStateConfigured,
			HealthState:       database.ConnectionHealthStateHealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  1,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	})
	require.NoError(t, err)

	err = resourceStore.StoreConnectionResourceSamples(ctx, []*ConnectionResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        connID,
			Namespace:         "root.acme.prod",
			Labels:            database.Labels{"env": "prod", "tier": "gold"},
			State:             database.ConnectionStateConfigured,
			HealthState:       database.ConnectionHealthStateUnhealthy,
			ConnectorID:       connectorID,
			ConnectorVersion:  2,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: sampledAt,
		},
	})
	require.NoError(t, err)

	start := sampledAt.Add(-time.Minute)
	end := sampledAt.Add(time.Minute)
	samples, err := resourceRetriever.ListConnectionResourceSamples(ctx, ResourceSampleQuery{
		Start:             &start,
		End:               &end,
		NamespaceMatchers: []string{"root.acme.**"},
		LabelSelector:     "tier=gold",
	})
	require.NoError(t, err)
	require.Len(t, samples, 1)

	got := samples[0]
	require.Equal(t, ResourceTypeConnection, got.ResourceType)
	require.Equal(t, connID, got.ResourceID)
	require.Equal(t, "root.acme.prod", got.Namespace)
	require.Equal(t, database.Labels{"env": "prod", "tier": "gold"}, got.Labels)
	require.Equal(t, database.ConnectionStateConfigured, got.State)
	require.Equal(t, database.ConnectionHealthStateUnhealthy, got.HealthState)
	require.Equal(t, connectorID, got.ConnectorID)
	require.Equal(t, uint64(2), got.ConnectorVersion)
	require.True(t, sampledAt.Equal(got.SampledAt))
	require.True(t, createdAt.Equal(got.ResourceCreatedAt))
	require.True(t, sampledAt.Equal(got.ResourceUpdatedAt))
	require.Nil(t, got.ResourceDeletedAt)
}

func TestResourceSamples_ActorSamples_IdempotentAndQueryable(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	resourceRetriever := retriever.(ResourceSampleRetriever)

	ctx := context.Background()
	sampledAt := time.Date(2026, 5, 25, 12, 15, 0, 0, time.UTC)
	actorID := apid.New(apid.PrefixActor)
	createdAt := sampledAt.Add(-2 * time.Hour)
	updatedAt := sampledAt.Add(-time.Minute)
	deletedAt := sampledAt.Add(time.Minute)

	err := resourceStore.StoreActorResourceSamples(ctx, []*ActorResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        actorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"team": "ops"},
			ExternalID:        "actor-1",
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	})
	require.NoError(t, err)

	err = resourceStore.StoreActorResourceSamples(ctx, []*ActorResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        actorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"team": "ops", "status": "deleted"},
			ExternalID:        "actor-1-renamed",
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: sampledAt,
			ResourceDeletedAt: &deletedAt,
		},
	})
	require.NoError(t, err)

	samples, err := resourceRetriever.ListActorResourceSamples(ctx, ResourceSampleQuery{
		ResourceIDs:   []apid.ID{actorID},
		LabelSelector: "status=deleted",
	})
	require.NoError(t, err)
	require.Len(t, samples, 1)

	got := samples[0]
	require.Equal(t, ResourceTypeActor, got.ResourceType)
	require.Equal(t, actorID, got.ResourceID)
	require.Equal(t, "root.acme", got.Namespace)
	require.Equal(t, database.Labels{"team": "ops", "status": "deleted"}, got.Labels)
	require.Equal(t, "actor-1-renamed", got.ExternalID)
	require.True(t, sampledAt.Equal(got.SampledAt))
	require.True(t, createdAt.Equal(got.ResourceCreatedAt))
	require.True(t, sampledAt.Equal(got.ResourceUpdatedAt))
	require.NotNil(t, got.ResourceDeletedAt)
	require.True(t, deletedAt.Equal(*got.ResourceDeletedAt))
}

func TestResourceSamples_AdditionalSamples_IdempotentAndQueryable(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	resourceStore := store.(ResourceSampleStore)
	resourceRetriever := retriever.(ResourceSampleRetriever)

	ctx := context.Background()
	sampledAt := time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	rateLimitID := apid.New(apid.PrefixRateLimit)
	createdAt := sampledAt.Add(-time.Hour)
	updatedAt := sampledAt.Add(-time.Minute)

	require.NoError(t, resourceStore.StoreConnectorResourceSamples(ctx, []*ConnectorResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        connectorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod", "kind": "connector"},
			State:             database.ConnectorVersionStateDraft,
			ConnectorVersion:  1,
			TotalVersions:     1,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	}))
	require.NoError(t, resourceStore.StoreConnectorResourceSamples(ctx, []*ConnectorResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        connectorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod", "kind": "connector"},
			State:             database.ConnectorVersionStatePrimary,
			ConnectorVersion:  2,
			TotalVersions:     2,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: sampledAt,
		},
	}))

	connectorSamples, err := resourceRetriever.ListConnectorResourceSamples(ctx, ResourceSampleQuery{
		NamespaceMatchers: []string{"root.acme"},
		LabelSelector:     "kind=connector",
	})
	require.NoError(t, err)
	require.Len(t, connectorSamples, 1)
	require.Equal(t, ResourceTypeConnector, connectorSamples[0].ResourceType)
	require.Equal(t, database.ConnectorVersionStatePrimary, connectorSamples[0].State)
	require.Equal(t, uint64(2), connectorSamples[0].ConnectorVersion)
	require.Equal(t, int64(2), connectorSamples[0].TotalVersions)

	require.NoError(t, resourceStore.StoreConnectorVersionResourceSamples(ctx, []*ConnectorVersionResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        connectorID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod", "kind": "connector-version"},
			State:             database.ConnectorVersionStatePrimary,
			ConnectorVersion:  2,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	}))
	connectorVersionSamples, err := resourceRetriever.ListConnectorVersionResourceSamples(ctx, ResourceSampleQuery{
		NamespaceMatchers: []string{"root.acme"},
		LabelSelector:     "kind=connector-version",
	})
	require.NoError(t, err)
	require.Len(t, connectorVersionSamples, 1)
	require.Equal(t, ResourceTypeConnectorVersion, connectorVersionSamples[0].ResourceType)
	require.Equal(t, database.ConnectorVersionStatePrimary, connectorVersionSamples[0].State)
	require.Equal(t, uint64(2), connectorVersionSamples[0].ConnectorVersion)

	require.NoError(t, resourceStore.StoreNamespaceResourceSamples(ctx, []*NamespaceResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        "root.acme",
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod", "kind": "namespace"},
			State:             database.NamespaceStateDestroying,
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	}))
	namespaceSamples, err := resourceRetriever.ListNamespaceResourceSamples(ctx, ResourceSampleQuery{
		NamespaceMatchers: []string{"root.acme"},
		LabelSelector:     "kind=namespace",
	})
	require.NoError(t, err)
	require.Len(t, namespaceSamples, 1)
	require.Equal(t, ResourceTypeNamespace, namespaceSamples[0].ResourceType)
	require.Equal(t, database.NamespaceStateDestroying, namespaceSamples[0].State)

	require.NoError(t, resourceStore.StoreRateLimitResourceSamples(ctx, []*RateLimitResourceSample{
		{
			SampledAt:         sampledAt,
			ResourceID:        rateLimitID,
			Namespace:         "root.acme",
			Labels:            database.Labels{"env": "prod", "kind": "rate-limit"},
			Mode:              "observe",
			ResourceCreatedAt: createdAt,
			ResourceUpdatedAt: updatedAt,
		},
	}))
	rateLimitSamples, err := resourceRetriever.ListRateLimitResourceSamples(ctx, ResourceSampleQuery{
		NamespaceMatchers: []string{"root.acme"},
		LabelSelector:     "kind=rate-limit",
	})
	require.NoError(t, err)
	require.Len(t, rateLimitSamples, 1)
	require.Equal(t, ResourceTypeRateLimit, rateLimitSamples[0].ResourceType)
	require.Equal(t, "observe", rateLimitSamples[0].Mode)
}
