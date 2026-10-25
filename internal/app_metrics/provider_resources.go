package app_metrics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

var connectionResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"state",
	"health_state",
	"connector_id",
	"connector_generation",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

var actorResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"external_id",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

var connectorResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"state",
	"connector_generation",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

var connectorGenerationResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"state",
	"connector_generation",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

var namespaceResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"state",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

var rateLimitResourceSampleColumns = []string{
	"sampled_at_ms",
	"resource_type",
	"resource_id",
	"namespace",
	"labels",
	"mode",
	"resource_created_at_ms",
	"resource_updated_at_ms",
	"resource_deleted_at_ms",
}

func (s *sqlRecordStore) StoreConnectionResourceSamples(
	ctx context.Context,
	samples []*ConnectionResourceSample,
) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.
		Insert(connectionResourceSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, connectionResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnection),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			string(sample.HealthState),
			sample.ConnectorID.String(),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		state = excluded.state,
		health_state = excluded.health_state,
		connector_id = excluded.connector_id,
		connector_generation = excluded.connector_generation,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build connection resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert connection resource samples: %w", err)
	}
	return nil
}

func (s *sqlRecordStore) StoreActorResourceSamples(
	ctx context.Context,
	samples []*ActorResourceSample,
) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(actorResourceSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, actorResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeActor),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			sample.ExternalID,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		external_id = excluded.external_id,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build actor resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert actor resource samples: %w", err)
	}
	return nil
}

func (s *sqlRecordStore) StoreConnectorResourceSamples(ctx context.Context, samples []*ConnectorResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(connectorResourceSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, connectorResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnector),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		state = excluded.state,
		connector_generation = excluded.connector_generation,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build connector resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert connector resource samples: %w", err)
	}
	return nil
}

func (s *sqlRecordStore) StoreConnectorGenerationResourceSamples(ctx context.Context, samples []*ConnectorGenerationResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(connectorGenerationSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, connectorGenerationResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnectorGeneration),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id, connector_generation) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		state = excluded.state,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build connector generation resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert connector generation resource samples: %w", err)
	}
	return nil
}

func (s *sqlRecordStore) StoreNamespaceResourceSamples(ctx context.Context, samples []*NamespaceResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(namespaceResourceSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, namespaceResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeNamespace),
			sample.ResourceID,
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		state = excluded.state,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build namespace resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert namespace resource samples: %w", err)
	}
	return nil
}

func (s *sqlRecordStore) StoreRateLimitResourceSamples(ctx context.Context, samples []*RateLimitResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(rateLimitResourceSamplesTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(append(append([]string{}, rateLimitResourceSampleColumns...), "ingested_at_unix_nano")...)

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		builder = builder.Values(
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeRateLimit),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			sample.Mode,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		)
	}

	builder = builder.Suffix(`ON CONFLICT (sampled_at_ms, resource_id) DO UPDATE SET
		resource_type = excluded.resource_type,
		namespace = excluded.namespace,
		labels = excluded.labels,
		mode = excluded.mode,
		resource_created_at_ms = excluded.resource_created_at_ms,
		resource_updated_at_ms = excluded.resource_updated_at_ms,
		resource_deleted_at_ms = excluded.resource_deleted_at_ms,
		ingested_at_unix_nano = excluded.ingested_at_unix_nano`)

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build rate limit resource sample insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert rate limit resource samples: %w", err)
	}
	return nil
}

func (s *clickhouseRecordStore) StoreConnectionResourceSamples(ctx context.Context, samples []*ConnectionResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		connectionResourceSamplesTable,
		strings.Join(connectionResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnection),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			string(sample.HealthState),
			sample.ConnectorID.String(),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse connection resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (s *clickhouseRecordStore) StoreActorResourceSamples(ctx context.Context, samples []*ActorResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		actorResourceSamplesTable,
		strings.Join(actorResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeActor),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			sample.ExternalID,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse actor resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (s *clickhouseRecordStore) StoreConnectorResourceSamples(ctx context.Context, samples []*ConnectorResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		connectorResourceSamplesTable,
		strings.Join(connectorResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnector),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse connector resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (s *clickhouseRecordStore) StoreConnectorGenerationResourceSamples(ctx context.Context, samples []*ConnectorGenerationResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		connectorGenerationSamplesTable,
		strings.Join(connectorGenerationResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeConnectorGeneration),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ConnectorGeneration,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse connector generation resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (s *clickhouseRecordStore) StoreNamespaceResourceSamples(ctx context.Context, samples []*NamespaceResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		namespaceResourceSamplesTable,
		strings.Join(namespaceResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeNamespace),
			sample.ResourceID,
			sample.Namespace,
			labelsVal,
			string(sample.State),
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse namespace resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (s *clickhouseRecordStore) StoreRateLimitResourceSamples(ctx context.Context, samples []*RateLimitResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (%s, ingested_at_unix_nano) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		rateLimitResourceSamplesTable,
		strings.Join(rateLimitResourceSampleColumns, ", "),
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	ingestedAt := time.Now().UnixNano()
	for _, sample := range samples {
		labelsVal, err := labelsValue(sample.Labels)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.SampledAt.UnixMilli(),
			defaultResourceType(sample.ResourceType, ResourceTypeRateLimit),
			sample.ResourceID.String(),
			sample.Namespace,
			labelsVal,
			sample.Mode,
			sample.ResourceCreatedAt.UnixMilli(),
			sample.ResourceUpdatedAt.UnixMilli(),
			nullableUnixMillis(sample.ResourceDeletedAt),
			ingestedAt,
		); err != nil {
			return fmt.Errorf("failed to insert clickhouse rate limit resource sample: %w", err)
		}
	}

	return tx.Commit()
}

func (r *sqlRecordRetriever) ListConnectionResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectionResourceSample, error) {
	return fetchConnectionResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListActorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ActorResourceSample, error) {
	return fetchActorResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListConnectorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectorResourceSample, error) {
	return fetchConnectorResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListConnectorGenerationResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectorGenerationResourceSample, error) {
	return fetchConnectorGenerationResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListNamespaceResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*NamespaceResourceSample, error) {
	return fetchNamespaceResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListRateLimitResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*RateLimitResourceSample, error) {
	return fetchRateLimitResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) QueryResourceMetrics(ctx context.Context, queries []ResourceMetricsQuery) ([]ResourceMetricSeries, error) {
	return executeResourceMetricsQueries(ctx, queries, &sqlResourceMetricFetcher{
		db:                r.db,
		placeholderFormat: r.placeholderFormat,
		provider:          r.provider,
	})
}

func (r *clickhouseRecordRetriever) ListConnectionResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectionResourceSample, error) {
	return fetchConnectionResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListActorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ActorResourceSample, error) {
	return fetchActorResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListConnectorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectorResourceSample, error) {
	return fetchConnectorResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListConnectorGenerationResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectorGenerationResourceSample, error) {
	return fetchConnectorGenerationResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListNamespaceResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*NamespaceResourceSample, error) {
	return fetchNamespaceResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListRateLimitResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*RateLimitResourceSample, error) {
	return fetchRateLimitResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) QueryResourceMetrics(ctx context.Context, queries []ResourceMetricsQuery) ([]ResourceMetricSeries, error) {
	return executeResourceMetricsQueries(ctx, queries, &sqlResourceMetricFetcher{
		db:                r.db,
		placeholderFormat: sq.Question,
		provider:          config.DatabaseProviderClickhouse,
	})
}

// resourceMetricSnapshotBatchSize caps how many snapshot times are bound into one aggregate query, keeping the IN
// list well under every provider's bound-parameter limit.
const resourceMetricSnapshotBatchSize = 500

// sqlResourceMetricFetcher implements resourceMetricFetcher for the SQL providers (Postgres, SQLite, and ClickHouse).
type sqlResourceMetricFetcher struct {
	db                *sql.DB
	placeholderFormat sq.PlaceholderFormat
	provider          config.DatabaseProvider
}

// fetchSampleTimes returns the distinct snapshot times in [from, to] for the query's metric table, ascending. Filters
// are intentionally not applied; see resourceMetricFetcher.
func (f *sqlResourceMetricFetcher) fetchSampleTimes(
	ctx context.Context,
	query ResourceMetricsQuery,
	from, to time.Time,
) ([]int64, error) {
	spec, ok := resourceMetricSpecs[query.Metric]
	if !ok {
		return nil, fmt.Errorf("unsupported resource metric %q", query.Metric)
	}

	sqlQuery, args, err := sq.Select("sampled_at_ms").
		Distinct().
		From(resourceSampleTableName(spec.table, f.provider)).
		Where(sq.GtOrEq{"sampled_at_ms": from.UnixMilli()}).
		Where(sq.LtOrEq{"sampled_at_ms": to.UnixMilli()}).
		OrderBy("sampled_at_ms ASC").
		PlaceholderFormat(f.placeholderFormat).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build resource snapshot times query: %w", err)
	}
	rows, err := f.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute resource snapshot times query: %w", err)
	}
	defer rows.Close()

	out := make([]int64, 0)
	for rows.Next() {
		var sampledAtMS int64
		if err := rows.Scan(&sampledAtMS); err != nil {
			return nil, fmt.Errorf("failed to scan resource snapshot time: %w", err)
		}
		out = append(out, sampledAtMS)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate resource snapshot times: %w", err)
	}
	return out, nil
}

// fetchAggregates returns distinct-resource counts per snapshot and group for the given snapshot times, applying the
// query's namespace and label filters. Large snapshot lists are queried in batches; each snapshot lands in exactly one
// batch, so the batch results can simply be concatenated.
func (f *sqlResourceMetricFetcher) fetchAggregates(
	ctx context.Context,
	query ResourceMetricsQuery,
	sampledAtMS []int64,
) ([]resourceMetricAggregate, error) {
	out := make([]resourceMetricAggregate, 0)
	for batchStart := 0; batchStart < len(sampledAtMS); batchStart += resourceMetricSnapshotBatchSize {
		batchEnd := min(batchStart+resourceMetricSnapshotBatchSize, len(sampledAtMS))
		rows, err := fetchResourceMetricAggregates(ctx, f.db, f.placeholderFormat, f.provider, query, sampledAtMS[batchStart:batchEnd])
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// fetchResourceMetricAggregates runs a resource metrics query against a SQL provider (Postgres, SQLite, or
// ClickHouse) and returns distinct-resource counts per snapshot and group, restricted to the given snapshot times.
//
// The inner SELECT DISTINCT reduces samples to one row per (snapshot, group values, resource identity), and the outer
// query counts those rows per (snapshot, group values). That keeps the result bounded by snapshots × groups no matter
// how many resources are sampled, de-duplicates a resource written twice for the same snapshot, and avoids
// COUNT(DISTINCT tuple), which SQLite does not support.
func fetchResourceMetricAggregates(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceMetricsQuery,
	sampledAtMS []int64,
) ([]resourceMetricAggregate, error) {
	spec, ok := resourceMetricSpecs[query.Metric]
	if !ok {
		return nil, fmt.Errorf("unsupported resource metric %q", query.Metric)
	}

	groupColumns := make([]string, 0, len(query.GroupBy))
	for _, groupBy := range query.GroupBy {
		column, ok := spec.groupByColumns[groupBy]
		if !ok {
			return nil, fmt.Errorf("unsupported resource group_by dimension %q for metric %q", groupBy, query.Metric)
		}
		groupColumns = append(groupColumns, column)
	}

	// The inner projection lists each column once; a column can be both a group-by dimension and part of the
	// resource identity (e.g. connector_generation), and ClickHouse rejects duplicate names in a subquery.
	innerColumns := []string{"sampled_at_ms"}
	seen := map[string]bool{}
	for _, column := range append(append([]string{}, groupColumns...), spec.identityColumns...) {
		if !seen[column] {
			seen[column] = true
			innerColumns = append(innerColumns, column)
		}
	}

	inner, err := applyResourceSampleQuery(
		sq.Select(innerColumns...).
			Distinct().
			From(resourceSampleTableName(spec.table, provider)).
			Where(sq.Eq{"sampled_at_ms": sampledAtMS}),
		provider,
		resourceMetricsSampleQuery(query),
	)
	if err != nil {
		return nil, err
	}

	outerGroup := append([]string{"sampled_at_ms"}, groupColumns...)
	outer := sq.Select(append(append([]string{}, outerGroup...), sqlInt64Expr(provider, "COUNT(*)"))...).
		FromSelect(inner, "distinct_samples").
		GroupBy(outerGroup...).
		PlaceholderFormat(placeholderFormat)

	sqlQuery, args, err := outer.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build resource metrics query: %w", err)
	}
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute resource metrics query: %w", err)
	}
	defer rows.Close()

	out := make([]resourceMetricAggregate, 0)
	for rows.Next() {
		var row resourceMetricAggregate
		// Group values are scanned generically because their column types differ by provider (e.g.
		// connector_generation is BIGINT on Postgres and UInt64 on ClickHouse).
		groupValues := make([]any, len(groupColumns))
		dest := make([]any, 0, len(groupColumns)+2)
		dest = append(dest, &row.SampledAtMS)
		for i := range groupValues {
			dest = append(dest, &groupValues[i])
		}
		dest = append(dest, &row.Count)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan resource metric row: %w", err)
		}

		row.Labels = make(map[string]string, len(query.GroupBy))
		for i, groupBy := range query.GroupBy {
			row.Labels[string(groupBy)] = resourceMetricLabelValue(groupValues[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate resource metric rows: %w", err)
	}
	return out, nil
}

// resourceMetricLabelValue renders a generically scanned group-by value as a label string. Drivers return text as
// string or []byte and integers as signed or unsigned ints depending on the provider.
func resourceMetricLabelValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func fetchConnectionResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*ConnectionResourceSample, error) {
	builder := sq.Select(connectionResourceSampleColumns...).
		From(resourceSampleTableName(connectionResourceSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*ConnectionResourceSample
	for rows.Next() {
		sample, err := scanConnectionResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func fetchActorResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*ActorResourceSample, error) {
	builder := sq.Select(actorResourceSampleColumns...).
		From(resourceSampleTableName(actorResourceSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*ActorResourceSample
	for rows.Next() {
		sample, err := scanActorResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func fetchConnectorResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*ConnectorResourceSample, error) {
	builder := sq.Select(connectorResourceSampleColumns...).
		From(resourceSampleTableName(connectorResourceSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*ConnectorResourceSample
	for rows.Next() {
		sample, err := scanConnectorResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func fetchConnectorGenerationResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*ConnectorGenerationResourceSample, error) {
	builder := sq.Select(connectorGenerationResourceSampleColumns...).
		From(resourceSampleTableName(connectorGenerationSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC", "connector_generation ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*ConnectorGenerationResourceSample
	for rows.Next() {
		sample, err := scanConnectorGenerationResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func fetchNamespaceResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*NamespaceResourceSample, error) {
	builder := sq.Select(namespaceResourceSampleColumns...).
		From(resourceSampleTableName(namespaceResourceSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*NamespaceResourceSample
	for rows.Next() {
		sample, err := scanNamespaceResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func fetchRateLimitResourceSamples(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query ResourceSampleQuery,
) ([]*RateLimitResourceSample, error) {
	builder := sq.Select(rateLimitResourceSampleColumns...).
		From(resourceSampleTableName(rateLimitResourceSamplesTable, provider)).
		PlaceholderFormat(placeholderFormat).
		OrderBy("sampled_at_ms ASC", "resource_id ASC")

	builder, err := applyResourceSampleQuery(builder, provider, query)
	if err != nil {
		return nil, err
	}

	rows, err := queryResourceSamples(ctx, db, builder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []*RateLimitResourceSample
	for rows.Next() {
		sample, err := scanRateLimitResourceSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func applyResourceSampleQuery(builder sq.SelectBuilder, provider config.DatabaseProvider, query ResourceSampleQuery) (sq.SelectBuilder, error) {
	if query.Start != nil {
		builder = builder.Where(sq.GtOrEq{"sampled_at_ms": query.Start.UnixMilli()})
	}
	if query.End != nil {
		builder = builder.Where(sq.LtOrEq{"sampled_at_ms": query.End.UnixMilli()})
	}
	if len(query.ResourceIDs) > 0 {
		ids := make([]string, 0, len(query.ResourceIDs))
		for _, id := range query.ResourceIDs {
			ids = append(ids, id.String())
		}
		builder = builder.Where(sq.Eq{"resource_id": ids})
	}
	if len(query.NamespaceMatchers) > 0 {
		for _, matcher := range query.NamespaceMatchers {
			if err := namespace.ValidateMatcher(matcher); err != nil {
				return builder, err
			}
		}
		builder = builder.Where(namespaceMatcherExpr(query.NamespaceMatchers))
	}
	if query.LabelSelector != "" {
		selector, err := database.ParseLabelSelector(query.LabelSelector)
		if err != nil {
			return builder, err
		}
		if len(selector) > 0 {
			builder = selector.ApplyToSqlBuilderWithProvider(builder, "labels", provider)
		}
	}
	if query.Limit > 0 {
		builder = builder.Limit(uint64(query.Limit))
	}
	return builder, nil
}

func queryResourceSamples(ctx context.Context, db *sql.DB, builder sq.SelectBuilder) (*sql.Rows, error) {
	sqlQuery, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build resource sample query: %w", err)
	}
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query resource samples: %w", err)
	}
	return rows, nil
}

func scanConnectionResourceSample(row interface{ Scan(dest ...any) error }) (*ConnectionResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	var resourceID, connectorID string
	sample := &ConnectionResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&resourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.State,
		&sample.HealthState,
		&connectorID,
		&sample.ConnectorGeneration,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan connection resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceID = apid.ID(resourceID)
	sample.ConnectorID = apid.ID(connectorID)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func scanActorResourceSample(row interface{ Scan(dest ...any) error }) (*ActorResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	var resourceID string
	sample := &ActorResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&resourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.ExternalID,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan actor resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceID = apid.ID(resourceID)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func scanConnectorResourceSample(row interface{ Scan(dest ...any) error }) (*ConnectorResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	var resourceID string
	sample := &ConnectorResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&resourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.State,
		&sample.ConnectorGeneration,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan connector resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceID = apid.ID(resourceID)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func scanConnectorGenerationResourceSample(row interface{ Scan(dest ...any) error }) (*ConnectorGenerationResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	var resourceID string
	sample := &ConnectorGenerationResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&resourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.State,
		&sample.ConnectorGeneration,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan connector generation resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceID = apid.ID(resourceID)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func scanNamespaceResourceSample(row interface{ Scan(dest ...any) error }) (*NamespaceResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	sample := &NamespaceResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&sample.ResourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.State,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan namespace resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func scanRateLimitResourceSample(row interface{ Scan(dest ...any) error }) (*RateLimitResourceSample, error) {
	var sampledAtMs, createdAtMs, updatedAtMs int64
	var deletedAtMs sql.NullInt64
	var resourceID string
	sample := &RateLimitResourceSample{}
	err := row.Scan(
		&sampledAtMs,
		&sample.ResourceType,
		&resourceID,
		&sample.Namespace,
		&sample.Labels,
		&sample.Mode,
		&createdAtMs,
		&updatedAtMs,
		&deletedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan rate limit resource sample: %w", err)
	}
	sample.SampledAt = unixMillis(sampledAtMs)
	sample.ResourceID = apid.ID(resourceID)
	sample.ResourceCreatedAt = unixMillis(createdAtMs)
	sample.ResourceUpdatedAt = unixMillis(updatedAtMs)
	sample.ResourceDeletedAt = nullableTime(deletedAtMs)
	return sample, nil
}

func labelsValue(labels database.Labels) (any, error) {
	labelsVal, err := labels.Value()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource sample labels: %w", err)
	}
	if labelsVal == nil {
		return "{}", nil
	}
	return labelsVal, nil
}

func defaultResourceType(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nullableUnixMillis(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}

func nullableTime(ms sql.NullInt64) *time.Time {
	if !ms.Valid {
		return nil
	}
	t := unixMillis(ms.Int64)
	return &t
}

func unixMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).In(time.UTC)
}

func resourceSampleTableName(table string, provider config.DatabaseProvider) string {
	if provider == config.DatabaseProviderClickhouse {
		return table + " FINAL"
	}
	return table
}

func namespaceMatcherExpr(matchers []string) sq.Sqlizer {
	or := sq.Or{}
	for _, matcher := range matchers {
		if strings.HasSuffix(matcher, ".**") {
			coreNamespace := matcher[:len(matcher)-3]
			or = append(or,
				sq.Eq{"namespace": coreNamespace},
				sq.Like{"namespace": coreNamespace + ".%"},
			)
		} else {
			or = append(or, sq.Eq{"namespace": matcher})
		}
	}
	return or
}

var _ ResourceSampleStore = (*sqlRecordStore)(nil)
var _ ResourceSampleStore = (*clickhouseRecordStore)(nil)
var _ ResourceSampleRetriever = (*sqlRecordRetriever)(nil)
var _ ResourceSampleRetriever = (*clickhouseRecordRetriever)(nil)
