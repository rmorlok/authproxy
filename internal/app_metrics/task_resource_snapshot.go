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

	logger.Info(
		"completed app metrics resource snapshot",
		"connections", connectionCount,
		"actors", actorCount,
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
