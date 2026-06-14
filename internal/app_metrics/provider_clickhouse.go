package app_metrics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	sq "github.com/Masterminds/squirrel"
	"github.com/golang-migrate/migrate/v4"
	chmigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type clickhouseRecordStore struct {
	db     *sql.DB
	cfg    *config.DatabaseClickhouse
	logger *slog.Logger
}

func NewClickhouseRecordStore(cfg *config.Database, logger *slog.Logger, dbOpts ...sqlh.Option) RecordStore {
	chCfg, ok := cfg.InnerVal.(*config.DatabaseClickhouse)
	if !ok {
		panic(fmt.Sprintf("expected *config.DatabaseClickhouse, got %T", cfg))
	}

	chOpts, err := chCfg.ToClickhouseOptions()
	if err != nil {
		panic(fmt.Errorf("failed to convert clickhouse config to options: %w", err))
	}

	db := sqlh.OpenInstrumentedConnector(clickhouse.Connector(chOpts), sqlh.DBSystemClickHouse, dbOpts...)
	s := &clickhouseRecordStore{
		db:     db,
		cfg:    chCfg,
		logger: logger.With("sub_component", "store"),
	}

	return s
}

func (s *clickhouseRecordStore) StoreRecords(ctx context.Context, records []*LogRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("failed to begin clickhouse transaction", "error", err)
		return err
	}

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (request_id, namespace, type, correlation_id, timestamp_ms, "+
			"duration_ms, connection_id, connector_id, connector_version, "+
			"method, host, scheme, path, "+
			"response_status_code, response_error, "+
			"request_http_version, request_size_bytes, request_mime_type, "+
			"response_http_version, response_size_bytes, response_mime_type, "+
			"internal_timeout, request_cancelled, full_request_recorded, "+
			"labels, response_source, rate_limit_id, rate_limit_mode, "+
			"rate_limit_bucket, rate_limit_matched, "+
			"request_body_skipped, response_body_skipped) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		entryRecordsTable,
	))
	if err != nil {
		s.logger.Error("failed to prepare clickhouse insert", "error", err)
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		labelsVal, _ := r.Labels.Value()
		if labelsVal == nil {
			labelsVal = "{}"
		}
		source := r.ResponseSource
		if source == "" {
			source = ResponseSourceUpstream
		}
		bucketJSON, err := marshalRateLimitBucket(r.RateLimitBucket)
		if err != nil {
			s.logger.Error("failed to marshal rate-limit bucket", "error", err, "entry_id", r.RequestId.String())
			continue
		}
		matchedJSON, err := marshalRateLimitMatched(r.RateLimitMatched)
		if err != nil {
			s.logger.Error("failed to marshal rate-limit matched", "error", err, "entry_id", r.RequestId.String())
			continue
		}
		_, err = stmt.ExecContext(ctx,
			r.RequestId.String(), r.Namespace, string(r.Type), r.CorrelationId,
			r.Timestamp.UnixMilli(), r.MillisecondDuration.Duration().Milliseconds(),
			r.ConnectionId.String(), r.ConnectorId.String(), r.ConnectorVersion,
			r.Method, r.Host, r.Scheme, r.Path,
			r.ResponseStatusCode, r.ResponseError,
			r.RequestHttpVersion, r.RequestSizeBytes, r.RequestMimeType,
			r.ResponseHttpVersion, r.ResponseSizeBytes, r.ResponseMimeType,
			r.InternalTimeout, r.RequestCancelled, r.FullRequestRecorded,
			labelsVal,
			string(source), r.RateLimitId.String(), r.RateLimitMode,
			bucketJSON, matchedJSON,
			string(r.RequestBodySkipped), string(r.ResponseBodySkipped),
		)
		if err != nil {
			s.logger.Error("failed to insert record into clickhouse", "error", err, "entry_id", r.RequestId.String())
		}
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit clickhouse transaction", "error", err)
	}

	return nil
}

func (s *clickhouseRecordStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	return s.StoreRecords(ctx, []*LogRecord{record})
}

func (s *clickhouseRecordStore) Migrate(ctx context.Context) error {
	s.logger.Info("running clickhouse app metrics migrations")
	defer s.logger.Info("clickhouse app metrics migrations complete")

	src, err := iofs.New(appMetricsMigrationsFs, "migrations/clickhouse")
	if err != nil {
		return fmt.Errorf("failed to load clickhouse app metrics migrations: %w", err)
	}

	var dbName string
	if s.cfg.Database != nil {
		dbName, err = s.cfg.Database.GetValue(ctx)
		if err != nil {
			return fmt.Errorf("failed to resolve clickhouse database name: %w", err)
		}
	}

	// The clickhouse migrate driver takes ownership of the *sql.DB it's given
	// and closes it when the migrator does — so we must hand it a dedicated
	// connection rather than the long-lived store handle.
	chOpts, err := s.cfg.ToClickhouseOptions()
	if err != nil {
		return fmt.Errorf("failed to derive clickhouse options for migration: %w", err)
	}
	migrationConn := sql.OpenDB(clickhouse.Connector(chOpts))

	driver, err := chmigrate.WithInstance(migrationConn, &chmigrate.Config{
		DatabaseName:          dbName,
		MigrationsTable:       appMetricsMigrationsTable,
		MultiStatementEnabled: true,
	})
	if err != nil {
		_ = migrationConn.Close()
		return fmt.Errorf("failed to setup clickhouse app metrics migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "clickhouse", driver)
	if err != nil {
		_ = migrationConn.Close()
		return fmt.Errorf("failed to setup clickhouse app metrics migrator: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			s.logger.Warn("failed to close clickhouse migrator", "source_err", sourceErr, "db_err", dbErr)
		}
	}()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			s.logger.Info("no clickhouse app metrics migrations required")
			return nil
		}
		return fmt.Errorf("failed to migrate clickhouse app metrics database: %w", err)
	}

	return nil
}

func (s *clickhouseRecordStore) Ping(ctx context.Context) bool {
	return s.db.PingContext(ctx) == nil
}

var _ RecordStore = (*clickhouseRecordStore)(nil)

// --- ClickHouse RecordRetriever ---

type clickhouseRecordRetriever struct {
	db              *sql.DB
	cfg             *config.DatabaseClickhouse
	cursorEncryptor pagination.CursorEncryptor
	logger          *slog.Logger
}

func NewClickhouseRecordRetriever(cfg *config.Database, cursorEncryptor pagination.CursorEncryptor, logger *slog.Logger, dbOpts ...sqlh.Option) RecordRetriever {
	chCfg, ok := cfg.InnerVal.(*config.DatabaseClickhouse)
	if !ok {
		panic(fmt.Sprintf("expected *config.DatabaseClickhouse, got %T", cfg))
	}

	options, err := chCfg.ToClickhouseOptions()
	if err != nil {
		panic(fmt.Errorf("failed to convert clickhouse config to options: %w", err))
	}

	db := sqlh.OpenInstrumentedConnector(clickhouse.Connector(options), sqlh.DBSystemClickHouse, dbOpts...)
	return &clickhouseRecordRetriever{
		db:              db,
		cfg:             chCfg,
		cursorEncryptor: cursorEncryptor,
		logger:          logger.With("sub_component", "retriever"),
	}
}

func (r *clickhouseRecordRetriever) GetRecord(ctx context.Context, id apid.ID) (*LogRecord, error) {
	query, args, err := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		Where(sq.Eq{"request_id": id.String()}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	er, err := scanLogRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get entry record from clickhouse: %w", err)
	}

	return er, nil
}

func (r *clickhouseRecordRetriever) NewListRequestsBuilder() ListRequestBuilder {
	return &clickhouseListRequestsBuilder{
		sqlListRequestsBuilder: sqlListRequestsBuilder{
			ListFilters:       ListFilters{LimitVal: 100},
			db:                r.db,
			cursorEncryptor:   r.cursorEncryptor,
			placeholderFormat: sq.Question,
			provider:          config.DatabaseProviderClickhouse,
		},
	}
}

func (r *clickhouseRecordRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	b := &clickhouseListRequestsBuilder{
		sqlListRequestsBuilder: sqlListRequestsBuilder{
			ListFilters:       ListFilters{LimitVal: 100},
			db:                r.db,
			cursorEncryptor:   r.cursorEncryptor,
			placeholderFormat: sq.Question,
			provider:          config.DatabaseProviderClickhouse,
		},
	}

	return b.fromCursor(ctx, cursor)
}

func (r *clickhouseRecordRetriever) QueryRequestEventMetrics(ctx context.Context, queries []RequestEventMetricsQuery) ([]RequestEventMetricSeries, error) {
	return executeRequestEventMetricsQueries(ctx, queries, func(ctx context.Context, query RequestEventMetricsQuery) ([]*LogRecord, error) {
		return fetchRequestEventMetricRecords(ctx, r.db, sq.Question, config.DatabaseProviderClickhouse, query)
	})
}

var _ RecordRetriever = (*clickhouseRecordRetriever)(nil)

// --- ClickHouse ListRequestBuilder (wraps SQL builder) ---

type clickhouseListRequestsBuilder struct {
	sqlListRequestsBuilder
}

func (l *clickhouseListRequestsBuilder) buildQuery() sq.SelectBuilder {
	builder := l.sqlListRequestsBuilder.buildQuery()

	// ClickHouse uses different regex syntax if needed
	if l.PathRegex != nil {
		// Override: ClickHouse uses match() function
		builder = sq.Select(entryRecordColumns...).
			From(entryRecordsTable)
		// Rebuild without the REGEXP clause and add ClickHouse-specific
		// For simplicity, reuse the parent builder (it uses standard SQL which ClickHouse supports)
		return l.sqlListRequestsBuilder.buildQuery()
	}

	return builder
}

func (l *clickhouseListRequestsBuilder) fromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	db := l.db
	cursorEncryptor := l.cursorEncryptor
	pf := l.placeholderFormat
	provider := l.provider

	parsed, err := pagination.ParseCursor[clickhouseListRequestsBuilder](ctx, l.cursorEncryptor, cursor)
	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db
	l.cursorEncryptor = cursorEncryptor
	l.placeholderFormat = pf
	l.provider = provider

	return l, nil
}

// Forward all ListRequestBuilder methods to ensure correct return types

func (l *clickhouseListRequestsBuilder) Limit(limit int32) ListRequestBuilder {
	l.sqlListRequestsBuilder.Limit(limit)
	return l
}

func (l *clickhouseListRequestsBuilder) OrderBy(field RequestOrderByField, by pagination.OrderBy) ListRequestBuilder {
	l.sqlListRequestsBuilder.OrderBy(field, by)
	return l
}

func (l *clickhouseListRequestsBuilder) ForNamespaceMatcher(matcher string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForNamespaceMatcher(matcher)
	return l
}

func (l *clickhouseListRequestsBuilder) ForNamespaceMatchers(matchers []string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForNamespaceMatchers(matchers)
	return l
}

func (l *clickhouseListRequestsBuilder) ForRequestType(requestType httpf.RequestType) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForRequestType(requestType)
	return l
}

func (l *clickhouseListRequestsBuilder) ForCorrelationId(correlationId string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForCorrelationId(correlationId)
	return l
}

func (l *clickhouseListRequestsBuilder) ForConnectionId(u apid.ID) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForConnectionId(u)
	return l
}

func (l *clickhouseListRequestsBuilder) ForConnectorType(t string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForConnectorType(t)
	return l
}

func (l *clickhouseListRequestsBuilder) ForConnectorId(u apid.ID) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForConnectorId(u)
	return l
}

func (l *clickhouseListRequestsBuilder) ForConnectorVersion(v uint64) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForConnectorVersion(v)
	return l
}

func (l *clickhouseListRequestsBuilder) ForMethod(method string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForMethod(method)
	return l
}

func (l *clickhouseListRequestsBuilder) ForStatusCode(s int) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForStatusCode(s)
	return l
}

func (l *clickhouseListRequestsBuilder) ForStatusCodeRangeInclusive(start, end int) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForStatusCodeRangeInclusive(start, end)
	return l
}

func (l *clickhouseListRequestsBuilder) ForParsedStatusCodeRange(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.ForParsedStatusCodeRange(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) ForPath(path string) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForPath(path)
	return l
}

func (l *clickhouseListRequestsBuilder) ForPathRegex(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.ForPathRegex(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) ForTimestampRange(start, end time.Time) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForTimestampRange(start, end)
	return l
}

func (l *clickhouseListRequestsBuilder) ForParsedTimestampRange(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.ForParsedTimestampRange(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) ForLabelSelector(selector string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.ForLabelSelector(selector)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) ForResponseSource(s ResponseSource) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForResponseSource(s)
	return l
}

func (l *clickhouseListRequestsBuilder) ForRateLimitId(id apid.ID) ListRequestBuilder {
	l.sqlListRequestsBuilder.ForRateLimitId(id)
	return l
}

var _ ListRequestExecutor = (*clickhouseListRequestsBuilder)(nil)
var _ ListRequestBuilder = (*clickhouseListRequestsBuilder)(nil)

// Suppress unused import warnings
var _ = strings.Contains
