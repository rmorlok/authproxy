package app_metrics

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
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

const (
	appMetricsMigrationsTable = "app_metrics_schema_migrations"
	entryRecordsTable         = "app_metrics_request_events"
)

// --- SQL RecordStore ---

type sqlRecordStore struct {
	db                *sql.DB
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
			"connector_generation",
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
			record.ConnectorGeneration,
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
	"duration_ms", "connection_id", "connector_id", "connector_generation",
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
		&durationMs, &connectionId, &connectorId, &er.ConnectorGeneration,
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
	return executeRequestEventMetricsQueries(ctx, queries, func(ctx context.Context, query RequestEventMetricsQuery) ([]requestEventMetricAggregate, error) {
		return fetchRequestEventMetricAggregates(ctx, r.db, r.placeholderFormat, r.provider, query)
	})
}

var _ RecordRetriever = (*sqlRecordRetriever)(nil)

// --- SQL ListRequestBuilder ---

var sqlOrderByColumns = map[RequestOrderByField]string{
	RequestOrderByTimestamp:           "timestamp_ms",
	RequestOrderByType:                "type",
	RequestOrderByCorrelationId:       "correlation_id",
	RequestOrderByConnectionId:        "connection_id",
	RequestOrderByConnectorType:       "type", // TODO: connector_type column
	RequestOrderByConnectorId:         "connector_id",
	RequestOrderByMethod:              "method",
	RequestOrderByPath:                "path",
	RequestOrderByResponseStatusCode:  "response_status_code",
	RequestOrderByConnectorGeneration: "connector_generation",
	RequestOrderByNamespace:           "namespace",
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

func (l *sqlListRequestsBuilder) ForConnectorGeneration(v uint64) ListRequestBuilder {
	l.ListFilters.SetConnectorGeneration(v)
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
	start, end, err := util.ParseHTTPStatusCodeRange(r)
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

	if l.ConnectorGeneration != nil {
		builder = builder.Where(sq.Eq{"connector_generation": *l.ConnectorGeneration})
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

// requestEventGroupByColumns maps each supported group-by dimension to the request-event column it groups on. Only
// columns from this allowlist are ever interpolated into metrics SQL.
var requestEventGroupByColumns = map[RequestEventGroupBy]string{
	RequestEventGroupByType:               "type",
	RequestEventGroupByMethod:             "method",
	RequestEventGroupByResponseStatusCode: "response_status_code",
	RequestEventGroupByResponseSource:     "response_source",
	RequestEventGroupByConnectorID:        "connector_id",
}

// sqlInt64Expr wraps an aggregate expression so it scans as a 64-bit integer on every provider: Postgres returns
// NUMERIC for SUM(bigint) and ClickHouse returns unsigned types for COUNT/SUM.
func sqlInt64Expr(provider config.DatabaseProvider, expr string) string {
	if provider == config.DatabaseProviderClickhouse {
		return "toInt64(" + expr + ")"
	}
	return "CAST(" + expr + " AS BIGINT)"
}

// fetchRequestEventMetricAggregates runs a request-event metrics query against a SQL provider (Postgres, SQLite, or
// ClickHouse) and returns rows aggregated by time bucket and the query's group-by dimensions.
//
// Aggregation happens in the database so the result size is bounded by buckets × groups rather than by the number of
// request events in the range; loading raw events made dashboard queries over busy namespaces exceed client timeouts.
// The exception is the p95 duration metric: nearest-rank percentiles are not portable across providers, so it selects
// a narrow (bucket, group columns, duration) projection per event and the percentile is computed in Go.
func fetchRequestEventMetricAggregates(
	ctx context.Context,
	db *sql.DB,
	placeholderFormat sq.PlaceholderFormat,
	provider config.DatabaseProvider,
	query RequestEventMetricsQuery,
) ([]requestEventMetricAggregate, error) {
	startMS := query.Start.UnixMilli()
	stepMS := query.Step.Milliseconds()

	// Align each event to the start of its bucket. The integers are derived from validated time values (not user
	// strings), and inlining them keeps the SELECT and GROUP BY expressions textually identical, which Postgres
	// requires when grouping by an expression. Modulo (rather than division) keeps the expression integer-valued on
	// ClickHouse, whose "/" operator always produces a float.
	bucketExpr := fmt.Sprintf("timestamp_ms - ((timestamp_ms - %d) %% %d)", startMS, stepMS)

	groupColumns := make([]string, 0, len(query.GroupBy))
	for _, groupBy := range query.GroupBy {
		column, ok := requestEventGroupByColumns[groupBy]
		if !ok {
			return nil, fmt.Errorf("unsupported request event group_by dimension %q", groupBy)
		}
		groupColumns = append(groupColumns, column)
	}

	perEvent := query.Metric == RequestEventMetricDurationP95MS
	columns := append([]string{bucketExpr}, groupColumns...)
	if perEvent {
		columns = append(columns, "duration_ms")
	} else {
		columns = append(columns,
			sqlInt64Expr(provider, "COUNT(*)"),
			sqlInt64Expr(provider, "SUM(CASE WHEN "+requestEventErrorSQL+" THEN 1 ELSE 0 END)"),
			sqlInt64Expr(provider, "SUM(duration_ms)"),
		)
	}

	builder := &sqlListRequestsBuilder{
		ListFilters:       requestEventMetricsFilters(query),
		db:                db,
		placeholderFormat: placeholderFormat,
		provider:          provider,
	}
	selectBuilder := builder.applyFilters(sq.Select(columns...).
		From(entryRecordsTable).
		PlaceholderFormat(placeholderFormat))
	if !perEvent {
		selectBuilder = selectBuilder.GroupBy(append([]string{bucketExpr}, groupColumns...)...)
	}

	sqlQuery, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build request event metrics query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request event metrics query: %w", err)
	}
	defer rows.Close()

	out := make([]requestEventMetricAggregate, 0)
	for rows.Next() {
		var row requestEventMetricAggregate
		var statusCode int64
		groupValues := make([]string, len(query.GroupBy))

		dest := make([]any, 0, len(columns))
		dest = append(dest, &row.BucketStartMS)
		for i, groupBy := range query.GroupBy {
			if groupBy == RequestEventGroupByResponseStatusCode {
				dest = append(dest, &statusCode)
			} else {
				dest = append(dest, &groupValues[i])
			}
		}
		if perEvent {
			row.Count = 1
			dest = append(dest, &row.DurationSumMS)
		} else {
			dest = append(dest, &row.Count, &row.ErrorCount, &row.DurationSumMS)
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan request event metric row: %w", err)
		}

		row.Labels = make(map[string]string, len(query.GroupBy))
		for i, groupBy := range query.GroupBy {
			value := groupValues[i]
			switch groupBy {
			case RequestEventGroupByResponseStatusCode:
				value = strconv.FormatInt(statusCode, 10)
			case RequestEventGroupByResponseSource:
				// Rows written before response_source existed may be blank; they were upstream responses.
				if value == "" {
					value = string(ResponseSourceUpstream)
				}
			}
			row.Labels[string(groupBy)] = value
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate request event metric rows: %w", err)
	}
	return out, nil
}

// Suppress unused import warnings
var _ = regexp.Compile
