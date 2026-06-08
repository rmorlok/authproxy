package app_metrics

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

//go:embed migrations/**/*.sql
var appMetricsMigrationsFs embed.FS

const entryRecordsTable = "app_metrics_request_events"

// --- SQL RecordStore ---

type sqlRecordStore struct {
	db                *sql.DB
	uri               string
	provider          config.DatabaseProvider
	cfg               *config.Database
	logger            *slog.Logger
	placeholderFormat sq.PlaceholderFormat
}

func NewSqlRecordStore(cfg *config.Database, logger *slog.Logger, opts ...sqlh.Option) RecordStore {
	db, err := sqlh.OpenInstrumentedSQL(cfg.GetDriver(), cfg.GetDsn(), dbSystemFor(cfg.GetProvider()), opts...)
	if err != nil {
		panic(fmt.Errorf("failed to open app metrics database: %w", err))
	}

	return &sqlRecordStore{
		db:                db,
		uri:               cfg.GetUri(),
		provider:          cfg.GetProvider(),
		cfg:               cfg,
		logger:            logger.With("sub_component", "store"),
		placeholderFormat: cfg.GetPlaceholderFormat(),
	}
}

func (s *sqlRecordStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	return s.StoreRecords(ctx, []*LogRecord{record})
}

func (s *sqlRecordStore) StoreRecords(ctx context.Context, records []*LogRecord) error {
	if len(records) == 0 {
		return nil
	}

	builder := sq.Insert(entryRecordsTable).
		PlaceholderFormat(s.placeholderFormat).
		Columns(
			"request_id",
			"namespace",
			"type",
			"correlation_id",
			"timestamp_ms",
			"duration_ms",
			"connection_id",
			"connector_id",
			"connector_version",
			"method",
			"host",
			"scheme",
			"path",
			"response_status_code",
			"response_error",
			"request_http_version",
			"request_size_bytes",
			"request_mime_type",
			"response_http_version",
			"response_size_bytes",
			"response_mime_type",
			"internal_timeout",
			"request_cancelled",
			"full_request_recorded",
			"labels",
			"response_source",
			"rate_limit_id",
			"rate_limit_mode",
			"rate_limit_bucket",
			"rate_limit_matched",
			"request_body_skipped",
			"response_body_skipped",
		)

	for _, record := range records {
		labelsVal, _ := record.Labels.Value()
		if labelsVal == nil {
			labelsVal = "{}"
		}
		source := record.ResponseSource
		if source == "" {
			source = ResponseSourceUpstream
		}
		bucketJSON, err := marshalRateLimitBucket(record.RateLimitBucket)
		if err != nil {
			return fmt.Errorf("failed to marshal rate-limit bucket: %w", err)
		}
		matchedJSON, err := marshalRateLimitMatched(record.RateLimitMatched)
		if err != nil {
			return fmt.Errorf("failed to marshal rate-limit matched: %w", err)
		}
		builder = builder.Values(
			record.RequestId.String(),
			record.Namespace,
			string(record.Type),
			record.CorrelationId,
			record.Timestamp.UnixMilli(),
			record.MillisecondDuration.Duration().Milliseconds(),
			record.ConnectionId.String(),
			record.ConnectorId.String(),
			record.ConnectorVersion,
			record.Method,
			record.Host,
			record.Scheme,
			record.Path,
			record.ResponseStatusCode,
			record.ResponseError,
			record.RequestHttpVersion,
			record.RequestSizeBytes,
			record.RequestMimeType,
			record.ResponseHttpVersion,
			record.ResponseSizeBytes,
			record.ResponseMimeType,
			record.InternalTimeout,
			record.RequestCancelled,
			record.FullRequestRecorded,
			labelsVal,
			string(source),
			record.RateLimitId.String(),
			record.RateLimitMode,
			bucketJSON,
			matchedJSON,
			string(record.RequestBodySkipped),
			string(record.ResponseBodySkipped),
		)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert entry records: %w", err)
	}

	return nil
}

func (s *sqlRecordStore) Migrate(ctx context.Context) error {
	provider := string(s.cfg.GetProvider())
	s.logger.Info("running app metrics database migrations", "provider", provider)
	defer s.logger.Info("app metrics database migrations complete")

	d, err := iofs.New(appMetricsMigrationsFs, fmt.Sprintf("migrations/%s", provider))
	if err != nil {
		return fmt.Errorf("failed to load app metrics migrations for '%s': %w", provider, err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, s.uri)
	if err != nil {
		return fmt.Errorf("failed to setup app metrics migrations: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			s.logger.Warn("failed to close migrator", "source_err", sourceErr, "db_err", dbErr)
		}
	}()

	err = m.Up()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			s.logger.Info("no app metrics migrations required")
			return nil
		}
		return fmt.Errorf("failed to migrate app metrics database: %w", err)
	}

	return nil
}

func (s *sqlRecordStore) Ping(ctx context.Context) bool {
	return s.db.PingContext(ctx) == nil
}

var _ RecordStore = (*sqlRecordStore)(nil)

// --- SQL RecordRetriever ---

type sqlRecordRetriever struct {
	db                *sql.DB
	cursorEncryptor   pagination.CursorEncryptor
	logger            *slog.Logger
	provider          config.DatabaseProvider
	placeholderFormat sq.PlaceholderFormat
}

func NewSqlRecordRetriever(cfg *config.Database, cursorEncryptor pagination.CursorEncryptor, logger *slog.Logger, opts ...sqlh.Option) RecordRetriever {
	db, err := sqlh.OpenInstrumentedSQL(cfg.GetDriver(), cfg.GetDsn(), dbSystemFor(cfg.GetProvider()), opts...)
	if err != nil {
		panic(fmt.Errorf("failed to open app metrics database for retrieval: %w", err))
	}

	return &sqlRecordRetriever{
		db:                db,
		cursorEncryptor:   cursorEncryptor,
		logger:            logger.With("sub_component", "retriever"),
		provider:          cfg.GetProvider(),
		placeholderFormat: cfg.GetPlaceholderFormat(),
	}
}

var entryRecordColumns = []string{
	"request_id", "namespace", "type", "correlation_id", "timestamp_ms",
	"duration_ms", "connection_id", "connector_id", "connector_version",
	"method", "host", "scheme", "path",
	"response_status_code", "response_error",
	"request_http_version", "request_size_bytes", "request_mime_type",
	"response_http_version", "response_size_bytes", "response_mime_type",
	"internal_timeout", "request_cancelled", "full_request_recorded",
	"labels",
	"response_source", "rate_limit_id", "rate_limit_mode",
	"rate_limit_bucket", "rate_limit_matched",
	"request_body_skipped", "response_body_skipped",
}

func scanLogRecord(row interface{ Scan(dest ...any) error }) (*LogRecord, error) {
	er := &LogRecord{}
	var requestId, connectionId, connectorId string
	var timestampMs, durationMs int64
	var responseSource, rateLimitId, rateLimitMode string
	var rateLimitBucket, rateLimitMatched []byte
	var requestBodySkipped, responseBodySkipped string

	err := row.Scan(
		&requestId, &er.Namespace, &er.Type, &er.CorrelationId, &timestampMs,
		&durationMs, &connectionId, &connectorId, &er.ConnectorVersion,
		&er.Method, &er.Host, &er.Scheme, &er.Path,
		&er.ResponseStatusCode, &er.ResponseError,
		&er.RequestHttpVersion, &er.RequestSizeBytes, &er.RequestMimeType,
		&er.ResponseHttpVersion, &er.ResponseSizeBytes, &er.ResponseMimeType,
		&er.InternalTimeout, &er.RequestCancelled, &er.FullRequestRecorded,
		&er.Labels,
		&responseSource, &rateLimitId, &rateLimitMode,
		&rateLimitBucket, &rateLimitMatched,
		&requestBodySkipped, &responseBodySkipped,
	)
	if err != nil {
		return nil, err
	}
	er.RequestBodySkipped = BodySkippedReason(requestBodySkipped)
	er.ResponseBodySkipped = BodySkippedReason(responseBodySkipped)

	er.RequestId = apid.ID(requestId)
	er.ConnectionId = apid.ID(connectionId)
	er.ConnectorId = apid.ID(connectorId)
	er.Timestamp = time.Unix(0, timestampMs*int64(time.Millisecond)).In(time.UTC)
	er.MillisecondDuration = MillisecondDuration(time.Duration(durationMs) * time.Millisecond)

	if responseSource == "" {
		responseSource = string(ResponseSourceUpstream)
	}
	er.ResponseSource = ResponseSource(responseSource)
	er.RateLimitId = apid.ID(rateLimitId)
	er.RateLimitMode = rateLimitMode
	if bucket, err := unmarshalRateLimitBucket(rateLimitBucket); err == nil {
		er.RateLimitBucket = bucket
	}
	if matched, err := unmarshalRateLimitMatched(rateLimitMatched); err == nil {
		er.RateLimitMatched = matched
	}

	return er, nil
}

func (r *sqlRecordRetriever) GetRecord(ctx context.Context, id apid.ID) (*LogRecord, error) {
	query, args, err := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		PlaceholderFormat(r.placeholderFormat).
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
		return nil, fmt.Errorf("failed to get entry record: %w", err)
	}

	return er, nil
}

func (r *sqlRecordRetriever) NewListRequestsBuilder() ListRequestBuilder {
	return &sqlListRequestsBuilder{
		ListFilters:       ListFilters{LimitVal: 100},
		db:                r.db,
		cursorEncryptor:   r.cursorEncryptor,
		placeholderFormat: r.placeholderFormat,
		provider:          r.provider,
	}
}

func (r *sqlRecordRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	b := &sqlListRequestsBuilder{
		ListFilters:       ListFilters{LimitVal: 100},
		db:                r.db,
		cursorEncryptor:   r.cursorEncryptor,
		placeholderFormat: r.placeholderFormat,
		provider:          r.provider,
	}

	return b.fromCursor(ctx, cursor)
}

func (r *sqlRecordRetriever) QueryRequestEventMetrics(ctx context.Context, queries []RequestEventMetricsQuery) ([]RequestEventMetricSeries, error) {
	return executeRequestEventMetricsQueries(ctx, queries, func(ctx context.Context, query RequestEventMetricsQuery) ([]*LogRecord, error) {
		return fetchRequestEventMetricRecords(ctx, r.db, r.placeholderFormat, r.provider, query)
	})
}

var _ RecordRetriever = (*sqlRecordRetriever)(nil)

// --- SQL ListRequestBuilder ---

var sqlOrderByColumns = map[RequestOrderByField]string{
	RequestOrderByTimestamp:          "timestamp_ms",
	RequestOrderByType:               "type",
	RequestOrderByCorrelationId:      "correlation_id",
	RequestOrderByConnectionId:       "connection_id",
	RequestOrderByConnectorType:      "type", // TODO: connector_type column
	RequestOrderByConnectorId:        "connector_id",
	RequestOrderByMethod:             "method",
	RequestOrderByPath:               "path",
	RequestOrderByResponseStatusCode: "response_status_code",
	RequestOrderByConnectorVersion:   "connector_version",
	RequestOrderByNamespace:          "namespace",
}

type sqlListRequestsBuilder struct {
	ListFilters
	db                *sql.DB                    `json:"-"`
	cursorEncryptor   pagination.CursorEncryptor `json:"-"`
	placeholderFormat sq.PlaceholderFormat       `json:"-"`
	provider          config.DatabaseProvider    `json:"-"`
}

func (l *sqlListRequestsBuilder) addError(e error) ListRequestBuilder {
	l.ListFilters.AddError(e)
	return l
}

func (l *sqlListRequestsBuilder) Limit(limit int32) ListRequestBuilder {
	l.ListFilters.SetLimit(limit)
	return l
}

func (l *sqlListRequestsBuilder) OrderBy(field RequestOrderByField, by pagination.OrderBy) ListRequestBuilder {
	l.ListFilters.SetOrderBy(field, by)
	return l
}

func (l *sqlListRequestsBuilder) ForNamespaceMatcher(matcher string) ListRequestBuilder {
	if err := l.ListFilters.SetNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	return l
}

func (l *sqlListRequestsBuilder) ForNamespaceMatchers(matchers []string) ListRequestBuilder {
	if err := l.ListFilters.SetNamespaceMatchers(matchers); err != nil {
		return l.addError(err)
	}
	return l
}

func (l *sqlListRequestsBuilder) ForRequestType(requestType httpf.RequestType) ListRequestBuilder {
	l.ListFilters.SetRequestType(requestType)
	return l
}

func (l *sqlListRequestsBuilder) ForCorrelationId(correlationId string) ListRequestBuilder {
	l.ListFilters.SetCorrelationId(correlationId)
	return l
}

func (l *sqlListRequestsBuilder) ForConnectionId(u apid.ID) ListRequestBuilder {
	l.ListFilters.SetConnectionId(u)
	return l
}

func (l *sqlListRequestsBuilder) ForConnectorType(t string) ListRequestBuilder {
	l.ListFilters.SetConnectorType(t)
	return l
}

func (l *sqlListRequestsBuilder) ForConnectorId(u apid.ID) ListRequestBuilder {
	l.ListFilters.SetConnectorId(u)
	return l
}

func (l *sqlListRequestsBuilder) ForConnectorVersion(v uint64) ListRequestBuilder {
	l.ListFilters.SetConnectorVersion(v)
	return l
}

func (l *sqlListRequestsBuilder) ForMethod(method string) ListRequestBuilder {
	l.ListFilters.SetMethod(method)
	return l
}

func (l *sqlListRequestsBuilder) ForStatusCode(s int) ListRequestBuilder {
	l.ListFilters.SetStatusCode(s)
	return l
}

func (l *sqlListRequestsBuilder) ForStatusCodeRangeInclusive(start, end int) ListRequestBuilder {
	l.ListFilters.SetStatusCodeRangeInclusive(start, end)
	return l
}

func (l *sqlListRequestsBuilder) ForParsedStatusCodeRange(r string) (ListRequestBuilder, error) {
	start, end, err := util.ParseIntegerRange(r, 100, 999)
	if err != nil {
		return nil, err
	}
	l.ListFilters.SetStatusCodeRangeInclusive(start, end)
	return l, nil
}

func (l *sqlListRequestsBuilder) ForPath(path string) ListRequestBuilder {
	l.ListFilters.SetPath(path)
	return l
}

func (l *sqlListRequestsBuilder) ForPathRegex(r string) (ListRequestBuilder, error) {
	if err := l.ListFilters.SetPathRegex(r); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *sqlListRequestsBuilder) ForTimestampRange(start, end time.Time) ListRequestBuilder {
	l.ListFilters.SetTimestampRange(start, end)
	return l
}

func (l *sqlListRequestsBuilder) ForParsedTimestampRange(r string) (ListRequestBuilder, error) {
	start, end, err := util.ParseTimestampRange(r)
	if err != nil {
		return nil, err
	}
	l.ListFilters.SetTimestampRange(start, end)
	return l, nil
}

func (l *sqlListRequestsBuilder) ForLabelSelector(selector string) (ListRequestBuilder, error) {
	_, err := database.ParseLabelSelector(selector)
	if err != nil {
		return nil, err
	}
	l.ListFilters.SetLabelSelector(selector)
	return l, nil
}

func (l *sqlListRequestsBuilder) ForResponseSource(s ResponseSource) ListRequestBuilder {
	l.ListFilters.SetResponseSource(s)
	return l
}

func (l *sqlListRequestsBuilder) ForRateLimitId(id apid.ID) ListRequestBuilder {
	l.ListFilters.SetRateLimitId(id)
	return l
}

func (l *sqlListRequestsBuilder) buildQuery() sq.SelectBuilder {
	builder := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		PlaceholderFormat(l.placeholderFormat)

	builder = l.applyFilters(builder)

	// Order by
	orderColumn := "timestamp_ms"
	orderDir := "DESC"

	if l.OrderByFieldVal != nil {
		if col, ok := sqlOrderByColumns[*l.OrderByFieldVal]; ok {
			orderColumn = col
		}
	}
	if l.OrderByVal != nil && *l.OrderByVal == pagination.OrderByAsc {
		orderDir = "ASC"
	}

	builder = builder.OrderBy(orderColumn + " " + orderDir)

	limit := l.LimitVal
	if limit <= 0 {
		limit = 100
	}
	builder = builder.Limit(uint64(limit + 1))
	builder = builder.Offset(uint64(l.Offset))

	return builder
}

func (l *sqlListRequestsBuilder) applyFilters(builder sq.SelectBuilder) sq.SelectBuilder {
	if l.RequestType != nil {
		builder = builder.Where(sq.Eq{"type": *l.RequestType})
	}

	if l.CorrelationId != nil {
		builder = builder.Where(sq.Eq{"correlation_id": *l.CorrelationId})
	}

	if l.ConnectionId != nil {
		builder = builder.Where(sq.Eq{"connection_id": l.ConnectionId.String()})
	}

	if l.ConnectorId != nil {
		builder = builder.Where(sq.Eq{"connector_id": l.ConnectorId.String()})
	}

	if l.ConnectorVersion != nil {
		builder = builder.Where(sq.Eq{"connector_version": *l.ConnectorVersion})
	}

	if l.Method != nil {
		builder = builder.Where(sq.Eq{"method": *l.Method})
	}

	if len(l.StatusCodeRangeInclusive) == 2 {
		builder = builder.Where(sq.GtOrEq{"response_status_code": l.StatusCodeRangeInclusive[0]}).
			Where(sq.LtOrEq{"response_status_code": l.StatusCodeRangeInclusive[1]})
	}

	if len(l.TimestampRange) == 2 {
		builder = builder.Where(sq.GtOrEq{"timestamp_ms": l.TimestampRange[0].UnixMilli()}).
			Where(sq.LtOrEq{"timestamp_ms": l.TimestampRange[1].UnixMilli()})
	}

	if l.Path != nil {
		builder = builder.Where(sq.Eq{"path": *l.Path})
	}

	if l.PathRegex != nil {
		if l.provider == config.DatabaseProviderPostgres {
			builder = builder.Where("path ~ ?", *l.PathRegex)
		} else {
			// SQLite uses REGEXP (requires extension, but standard pattern)
			builder = builder.Where("path REGEXP ?", *l.PathRegex)
		}
	}

	if len(l.NamespaceMatchers) > 0 {
		or := sq.Or{}
		for _, matcher := range l.NamespaceMatchers {
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
		builder = builder.Where(or)
	}

	if l.ConnectorType != nil {
		ct := *l.ConnectorType
		if strings.Contains(ct, "*") || strings.Contains(ct, "%") {
			ct = strings.ReplaceAll(ct, "*", "%")
			builder = builder.Where(sq.Like{"type": ct})
		} else {
			builder = builder.Where(sq.Eq{"type": ct})
		}
	}

	if l.LabelSelector != nil {
		selector, err := database.ParseLabelSelector(*l.LabelSelector)
		if err == nil && len(selector) > 0 {
			builder = selector.ApplyToSqlBuilderWithProvider(builder, "labels", l.provider)
		}
	}

	if l.ResponseSource != nil {
		builder = builder.Where(sq.Eq{"response_source": *l.ResponseSource})
	}

	if l.RateLimitId != nil {
		builder = builder.Where(sq.Eq{"rate_limit_id": l.RateLimitId.String()})
	}

	return builder
}

func (l *sqlListRequestsBuilder) fromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	db := l.db
	cursorEncryptor := l.cursorEncryptor
	pf := l.placeholderFormat
	provider := l.provider

	parsed, err := pagination.ParseCursor[sqlListRequestsBuilder](ctx, l.cursorEncryptor, cursor)
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

func (l *sqlListRequestsBuilder) FetchPage(ctx context.Context) pagination.PageResult[*LogRecord] {
	if err := l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[*LogRecord]{Error: err}
	}

	if err := l.Validate(); err != nil {
		return pagination.PageResult[*LogRecord]{Error: err}
	}

	builder := l.buildQuery()
	query, args, err := builder.ToSql()
	if err != nil {
		return pagination.PageResult[*LogRecord]{Error: fmt.Errorf("failed to build list query: %w", err)}
	}

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return pagination.PageResult[*LogRecord]{Error: fmt.Errorf("failed to execute list query: %w", err)}
	}
	defer rows.Close()

	entries := make([]*LogRecord, 0)
	for rows.Next() {
		er, err := scanLogRecord(rows)
		if err != nil {
			return pagination.PageResult[*LogRecord]{Error: fmt.Errorf("failed to scan entry record: %w", err)}
		}
		entries = append(entries, er)
	}

	limit := l.LimitVal
	if limit <= 0 {
		limit = 100
	}

	l.Offset = l.Offset + int32(len(entries)) - 1

	cursorStr := ""
	hasMore := int32(len(entries)) > limit
	if hasMore {
		cursorStr, err = pagination.MakeCursor(ctx, l.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[*LogRecord]{Error: err}
		}
	}

	return pagination.PageResult[*LogRecord]{
		HasMore: hasMore,
		Results: entries[:util.MinInt32(limit, int32(len(entries)))],
		Cursor:  cursorStr,
	}
}

func (l *sqlListRequestsBuilder) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[*LogRecord]) error {
	var err error
	keepGoing := pagination.Continue
	hasMore := true

	for err == nil && hasMore && bool(keepGoing) {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

var _ ListRequestExecutor = (*sqlListRequestsBuilder)(nil)
var _ ListRequestBuilder = (*sqlListRequestsBuilder)(nil)

func fetchRequestEventMetricRecords(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query RequestEventMetricsQuery,
) ([]*LogRecord, error) {
	builder := &sqlListRequestsBuilder{
		ListFilters:       requestEventMetricsFilters(query),
		db:                db,
		placeholderFormat: placeholderFormat,
		provider:          provider,
	}
	selectBuilder := builder.applyFilters(sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		PlaceholderFormat(placeholderFormat)).
		OrderBy("timestamp_ms ASC")

	sqlQuery, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build request event metrics query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request event metrics query: %w", err)
	}
	defer rows.Close()

	records := make([]*LogRecord, 0)
	for rows.Next() {
		record, err := scanLogRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request event metric record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate request event metric records: %w", err)
	}
	return records, nil
}

// Suppress unused import warnings
var _ = regexp.Compile
