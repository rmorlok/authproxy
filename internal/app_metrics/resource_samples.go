package app_metrics

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

const (
	connectionResourceSamplesTable = "app_metrics_connection_resource_samples"
	actorResourceSamplesTable      = "app_metrics_actor_resource_samples"
	connectorResourceSamplesTable  = "app_metrics_connector_resource_samples"
	connectorVersionSamplesTable   = "app_metrics_connector_version_resource_samples"
	namespaceResourceSamplesTable  = "app_metrics_namespace_resource_samples"
	rateLimitResourceSamplesTable  = "app_metrics_rate_limit_resource_samples"

	ResourceTypeConnection       = "connection"
	ResourceTypeActor            = "actor"
	ResourceTypeConnector        = "connector"
	ResourceTypeConnectorVersion = "connector_version"
	ResourceTypeNamespace        = "namespace"
	ResourceTypeRateLimit        = "rate_limit"
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

// ConnectorResourceSample is a point-in-time snapshot of a connector collapsed
// across versions.
type ConnectorResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        apid.ID
	Namespace         string
	Labels            database.Labels
	State             database.ConnectorVersionState
	ConnectorVersion  uint64
	TotalVersions     int64
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}

// ConnectorVersionResourceSample is a point-in-time snapshot of a connector
// version.
type ConnectorVersionResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        apid.ID
	Namespace         string
	Labels            database.Labels
	State             database.ConnectorVersionState
	ConnectorVersion  uint64
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}

// NamespaceResourceSample is a point-in-time snapshot of a namespace.
type NamespaceResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        string
	Namespace         string
	Labels            database.Labels
	State             database.NamespaceState
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}

// RateLimitResourceSample is a point-in-time snapshot of a rate limiter.
type RateLimitResourceSample struct {
	SampledAt         time.Time
	ResourceType      string
	ResourceID        apid.ID
	Namespace         string
	Labels            database.Labels
	Mode              string
	ResourceCreatedAt time.Time
	ResourceUpdatedAt time.Time
	ResourceDeletedAt *time.Time
}
