package app_metrics

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

const (
	connectionResourceSamplesTable = "app_metrics_connection_resource_samples"
	actorResourceSamplesTable      = "app_metrics_actor_resource_samples"

	ResourceTypeConnection = "connection"
	ResourceTypeActor      = "actor"
)

// ResourceSampleQuery filters point-in-time resource samples.
type ResourceSampleQuery struct {
	Start             *time.Time
	End               *time.Time
	ResourceIDs       []apid.ID
	NamespaceMatchers []string
	LabelSelector     string
	Limit             int
}

// ConnectionResourceSample is a point-in-time snapshot of a connection for
// resource metrics. Samples are keyed by SampledAt + ResourceID so rerunning a
// snapshot bucket is idempotent.
type ConnectionResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        apid.ID
	Namespace         string
	Labels            database.Labels
	State             database.ConnectionState
	HealthState       database.ConnectionHealthState
	ConnectorID       apid.ID
	ConnectorVersion  uint64
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}

// ActorResourceSample is a point-in-time snapshot of an actor for resource
// metrics.
type ActorResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        apid.ID
	Namespace         string
	Labels            database.Labels
	ExternalID        string
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}
