package app_metrics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const (
	taskTypeResourceSnapshot  = "app_metrics:resource_snapshot"
	resourceSnapshotBatchSize = 1000
)

// ResourceSnapshotTaskHandler snapshots live resources into app_metrics storage.
type ResourceSnapshotTaskHandler struct {
	db     database.DB
	store  ResourceSampleStore
	cfg    *sconfig.AppMetrics
	logger *slog.Logger
}

func NewResourceSnapshotTaskHandler(
	db database.DB,
	store ResourceSampleStore,
	cfg *sconfig.AppMetrics,
	logger *slog.Logger,
) *ResourceSnapshotTaskHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &ResourceSnapshotTaskHandler{
		db:     db,
		store:  store,
		cfg:    cfg,
		logger: logger.With("component", "app_metrics_resource_snapshot"),
	}
}

func NewResourceSnapshotTask() *asynq.Task {
	return asynq.NewTask(taskTypeResourceSnapshot, nil)
}

func (h *ResourceSnapshotTaskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeResourceSnapshot, h.runResourceSnapshotTask)
}

func (h *ResourceSnapshotTaskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	if h == nil || h.cfg == nil || h.cfg.Database == nil {
		return nil
	}

	interval := h.cfg.GetResourceSnapshotInterval()
	if interval <= 0 {
		interval = (&sconfig.AppMetrics{}).GetResourceSnapshotInterval()
	}

	return []*asynq.PeriodicTaskConfig{{
		Cronspec: fmt.Sprintf("@every %s", interval),
		Task:     NewResourceSnapshotTask(),
	}}
}

func (h *ResourceSnapshotTaskHandler) runResourceSnapshotTask(ctx context.Context, _ *asynq.Task) error {
	now := apctx.GetClock(ctx).Now()
	return h.SnapshotResources(ctx, now)
}

func (h *ResourceSnapshotTaskHandler) SnapshotResources(ctx context.Context, at time.Time) error {
	if h == nil {
		return fmt.Errorf("resource snapshot task handler is nil")
	}
	if h.db == nil {
		return fmt.Errorf("resource snapshot database is required")
	}
	if h.store == nil {
		return fmt.Errorf("resource snapshot store is required")
	}

	interval := h.cfg.GetResourceSnapshotInterval()
	if interval <= 0 {
		interval = (&sconfig.AppMetrics{}).GetResourceSnapshotInterval()
	}
	sampledAt := at.UTC().Truncate(interval)

	start := time.Now()
	logger := h.logger.With("sampled_at", sampledAt, "interval", interval)
	logger.Info("starting app metrics resource snapshot")

	connectionCount, err := h.snapshotConnections(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot connection resources", "error", err, "duration", time.Since(start))
		return err
	}

	actorCount, err := h.snapshotActors(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot actor resources", "error", err, "duration", time.Since(start))
		return err
	}

	connectorCount, err := h.snapshotConnectors(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot connector resources", "error", err, "duration", time.Since(start))
		return err
	}

	connectorVersionCount, err := h.snapshotConnectorVersions(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot connector version resources", "error", err, "duration", time.Since(start))
		return err
	}

	namespaceCount, err := h.snapshotNamespaces(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot namespace resources", "error", err, "duration", time.Since(start))
		return err
	}

	rateLimitCount, err := h.snapshotRateLimits(ctx, sampledAt)
	if err != nil {
		logger.Error("failed to snapshot rate limit resources", "error", err, "duration", time.Since(start))
		return err
	}

	logger.Info(
		"completed app metrics resource snapshot",
		"connections", connectionCount,
		"actors", actorCount,
		"connectors", connectorCount,
		"connector_versions", connectorVersionCount,
		"namespaces", namespaceCount,
		"rate_limits", rateLimitCount,
		"duration", time.Since(start),
	)
	return nil
}

func (h *ResourceSnapshotTaskHandler) snapshotConnections(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListConnectionsBuilder().
		WithDeletedHandling(database.DeletedHandlingExclude).
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[database.Connection]) (pagination.KeepGoing, error) {
			samples := make([]*ConnectionResourceSample, 0, len(page.Results))
			for _, conn := range page.Results {
				conn := conn
				samples = append(samples, &ConnectionResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeConnection,
					ResourceID:        conn.Id,
					Namespace:         conn.Namespace,
					Labels:            conn.Labels,
					State:             conn.State,
					HealthState:       conn.HealthState,
					ConnectorID:       conn.ConnectorId,
					ConnectorVersion:  conn.ConnectorVersion,
					ResourceCreatedAt: conn.CreatedAt,
					ResourceUpdatedAt: conn.UpdatedAt,
					ResourceDeletedAt: conn.DeletedAt,
				})
			}
			if err := h.store.StoreConnectionResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate connection resources for app metrics snapshot: %w", err)
	}
	return total, nil
}

func (h *ResourceSnapshotTaskHandler) snapshotConnectors(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListConnectorsBuilder().
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[database.Connector]) (pagination.KeepGoing, error) {
			samples := make([]*ConnectorResourceSample, 0, len(page.Results))
			for _, connector := range page.Results {
				connector := connector
				samples = append(samples, &ConnectorResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeConnector,
					ResourceID:        connector.Id,
					Namespace:         connector.Namespace,
					Labels:            connector.Labels,
					State:             connector.State,
					ConnectorVersion:  connector.Version,
					TotalVersions:     connector.TotalVersions,
					ResourceCreatedAt: connector.CreatedAt,
					ResourceUpdatedAt: connector.UpdatedAt,
					ResourceDeletedAt: connector.DeletedAt,
				})
			}
			if err := h.store.StoreConnectorResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate connector resources for app metrics snapshot: %w", err)
	}
	return total, nil
}

func (h *ResourceSnapshotTaskHandler) snapshotConnectorVersions(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListConnectorVersionsBuilder().
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[database.ConnectorVersion]) (pagination.KeepGoing, error) {
			samples := make([]*ConnectorVersionResourceSample, 0, len(page.Results))
			for _, connectorVersion := range page.Results {
				connectorVersion := connectorVersion
				samples = append(samples, &ConnectorVersionResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeConnectorVersion,
					ResourceID:        connectorVersion.Id,
					Namespace:         connectorVersion.Namespace,
					Labels:            connectorVersion.Labels,
					State:             connectorVersion.State,
					ConnectorVersion:  connectorVersion.Version,
					ResourceCreatedAt: connectorVersion.CreatedAt,
					ResourceUpdatedAt: connectorVersion.UpdatedAt,
					ResourceDeletedAt: connectorVersion.DeletedAt,
				})
			}
			if err := h.store.StoreConnectorVersionResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate connector version resources for app metrics snapshot: %w", err)
	}
	return total, nil
}

func (h *ResourceSnapshotTaskHandler) snapshotNamespaces(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListNamespacesBuilder().
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[database.Namespace]) (pagination.KeepGoing, error) {
			samples := make([]*NamespaceResourceSample, 0, len(page.Results))
			for _, ns := range page.Results {
				ns := ns
				samples = append(samples, &NamespaceResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeNamespace,
					ResourceID:        ns.Path,
					Namespace:         ns.Path,
					Labels:            ns.Labels,
					State:             ns.State,
					ResourceCreatedAt: ns.CreatedAt,
					ResourceUpdatedAt: ns.UpdatedAt,
					ResourceDeletedAt: ns.DeletedAt,
				})
			}
			if err := h.store.StoreNamespaceResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate namespace resources for app metrics snapshot: %w", err)
	}
	return total, nil
}

func (h *ResourceSnapshotTaskHandler) snapshotRateLimits(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListRateLimitsBuilder().
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[database.RateLimit]) (pagination.KeepGoing, error) {
			samples := make([]*RateLimitResourceSample, 0, len(page.Results))
			for _, rateLimit := range page.Results {
				rateLimit := rateLimit
				samples = append(samples, &RateLimitResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeRateLimit,
					ResourceID:        rateLimit.Id,
					Namespace:         rateLimit.Namespace,
					Labels:            rateLimit.Labels,
					Mode:              string(rateLimit.Definition.EffectiveMode()),
					ResourceCreatedAt: rateLimit.CreatedAt,
					ResourceUpdatedAt: rateLimit.UpdatedAt,
					ResourceDeletedAt: rateLimit.DeletedAt,
				})
			}
			if err := h.store.StoreRateLimitResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate rate limit resources for app metrics snapshot: %w", err)
	}
	return total, nil
}

func (h *ResourceSnapshotTaskHandler) snapshotActors(ctx context.Context, sampledAt time.Time) (int, error) {
	total := 0
	err := h.db.ListActorsBuilder().
		Limit(resourceSnapshotBatchSize).
		Enumerate(ctx, func(page pagination.PageResult[*database.Actor]) (pagination.KeepGoing, error) {
			samples := make([]*ActorResourceSample, 0, len(page.Results))
			for _, actor := range page.Results {
				samples = append(samples, &ActorResourceSample{
					SampledAt:         sampledAt,
					ResourceType:      ResourceTypeActor,
					ResourceID:        actor.Id,
					Namespace:         actor.Namespace,
					Labels:            actor.Labels,
					ExternalID:        actor.ExternalId,
					ResourceCreatedAt: actor.CreatedAt,
					ResourceUpdatedAt: actor.UpdatedAt,
					ResourceDeletedAt: actor.DeletedAt,
				})
			}
			if err := h.store.StoreActorResourceSamples(ctx, samples); err != nil {
				return pagination.Stop, err
			}
			total += len(samples)
			return pagination.Continue, nil
		})
	if err != nil {
		return total, fmt.Errorf("failed to enumerate actor resources for app metrics snapshot: %w", err)
	}
	return total, nil
}
