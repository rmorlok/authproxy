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
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
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
	"connector_version",
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

func (s *sqlRecordStore) StoreConnectionResourceSamples(ctx context.Context, samples []*ConnectionResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	builder := sq.Insert(connectionResourceSamplesTable).
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
			sample.ConnectorVersion,
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
		connector_version = excluded.connector_version,
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

func (s *sqlRecordStore) StoreActorResourceSamples(ctx context.Context, samples []*ActorResourceSample) error {
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
			sample.ConnectorVersion,
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

func (r *sqlRecordRetriever) ListConnectionResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectionResourceSample, error) {
	return fetchConnectionResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *sqlRecordRetriever) ListActorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ActorResourceSample, error) {
	return fetchActorResourceSamples(ctx, r.db, r.placeholderFormat, r.provider, query)
}

func (r *clickhouseRecordRetriever) ListConnectionResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectionResourceSample, error) {
	return fetchConnectionResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
}

func (r *clickhouseRecordRetriever) ListActorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ActorResourceSample, error) {
	return fetchActorResourceSamples(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
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
			if err := aschema.ValidateNamespaceMatcher(matcher); err != nil {
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
		&sample.ConnectorVersion,
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
