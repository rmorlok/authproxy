package request_log

import (
	"context"
	"database/sql"
	"embed"
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
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

//go:embed migrations/**/*.sql
var httpLogMigrationsFs embed.FS

const entryRecordsTable = "http_log_entry_records"

// --- SQL RecordStore ---

type sqlRecordStore struct {
	db                *sql.DB
	uri               string
	provider          config.DatabaseProvider
	cfg               *config.Database
	logger            *slog.Logger
	placeholderFormat sq.PlaceholderFormat
}

func NewSqlRecordStore(cfg *config.Database, logger *slog.Logger) RecordStore {
	db, err := sql.Open(cfg.GetDriver(), cfg.GetDsn())
	if err != nil {
		panic(errors.Wrap(err, "failed to open http logging database"))
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
		)

	for _, record := range records {
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
		)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build insert query")
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to insert entry records")
	}

	return nil
}

func (s *sqlRecordStore) Migrate(ctx context.Context) error {
	provider := string(s.cfg.GetProvider())
	s.logger.Info("running http log database migrations", "provider", provider)
	defer s.logger.Info("http log database migrations complete")

	d, err := iofs.New(httpLogMigrationsFs, fmt.Sprintf("migrations/%s", provider))
	if err != nil {
		return errors.Wrapf(err, "failed to load http log migrations for '%s'", provider)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, s.uri)
	if err != nil {
		return errors.Wrap(err, "failed to setup http log migrations")
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
			s.logger.Info("no http log migrations required")
			return nil
		}
		return errors.Wrap(err, "failed to migrate http log database")
	}

	return nil
}

var _ RecordStore = (*sqlRecordStore)(nil)

// --- SQL RecordRetriever ---

type sqlRecordRetriever struct {
	db                *sql.DB
	cursorKey         config.KeyDataType
	logger            *slog.Logger
	provider          config.DatabaseProvider
	placeholderFormat sq.PlaceholderFormat
}

func NewSqlRecordRetriever(cfg *config.Database, cursorKey config.KeyDataType, logger *slog.Logger) RecordRetriever {
	db, err := sql.Open(cfg.GetDriver(), cfg.GetDsn())
	if err != nil {
		panic(errors.Wrap(err, "failed to open http logging database for retrieval"))
	}

	return &sqlRecordRetriever{
		db:                db,
		cursorKey:         cursorKey,
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
}

func scanLogRecord(row interface{ Scan(dest ...any) error }) (*LogRecord, error) {
	er := &LogRecord{}
	var requestId, connectionId, connectorId string
	var timestampMs, durationMs int64

	err := row.Scan(
		&requestId, &er.Namespace, &er.Type, &er.CorrelationId, &timestampMs,
		&durationMs, &connectionId, &connectorId, &er.ConnectorVersion,
		&er.Method, &er.Host, &er.Scheme, &er.Path,
		&er.ResponseStatusCode, &er.ResponseError,
		&er.RequestHttpVersion, &er.RequestSizeBytes, &er.RequestMimeType,
		&er.ResponseHttpVersion, &er.ResponseSizeBytes, &er.ResponseMimeType,
		&er.InternalTimeout, &er.RequestCancelled, &er.FullRequestRecorded,
	)
	if err != nil {
		return nil, err
	}

	er.RequestId = apid.ID(requestId)
	er.ConnectionId = apid.ID(connectionId)
	er.ConnectorId = apid.ID(connectorId)
	er.Timestamp = time.Unix(0, timestampMs*int64(time.Millisecond)).In(time.UTC)
	er.MillisecondDuration = MillisecondDuration(time.Duration(durationMs) * time.Millisecond)

	return er, nil
}

func (r *sqlRecordRetriever) GetRecord(ctx context.Context, id apid.ID) (*LogRecord, error) {
	query, args, err := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		PlaceholderFormat(r.placeholderFormat).
		Where(sq.Eq{"request_id": id.String()}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build select query")
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	er, err := scanLogRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to get entry record")
	}

	return er, nil
}

func (r *sqlRecordRetriever) NewListRequestsBuilder() ListRequestBuilder {
	return &sqlListRequestsBuilder{
		ListFilters:       ListFilters{LimitVal: 100},
		db:                r.db,
		cursorKey:         r.cursorKey,
		placeholderFormat: r.placeholderFormat,
		provider:          r.provider,
	}
}

func (r *sqlRecordRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	b := &sqlListRequestsBuilder{
		ListFilters:       ListFilters{LimitVal: 100},
		db:                r.db,
		cursorKey:         r.cursorKey,
		placeholderFormat: r.placeholderFormat,
		provider:          r.provider,
	}

	return b.fromCursor(ctx, cursor)
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
	db                *sql.DB                 `json:"-"`
	cursorKey         config.KeyDataType      `json:"-"`
	placeholderFormat sq.PlaceholderFormat    `json:"-"`
	provider          config.DatabaseProvider `json:"-"`
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

func (l *sqlListRequestsBuilder) WithNamespaceMatcher(matcher string) ListRequestBuilder {
	if err := l.ListFilters.SetNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	}
	return l
}

func (l *sqlListRequestsBuilder) WithNamespaceMatchers(matchers []string) ListRequestBuilder {
	if err := l.ListFilters.SetNamespaceMatchers(matchers); err != nil {
		return l.addError(err)
	}
	return l
}

func (l *sqlListRequestsBuilder) WithRequestType(requestType httpf.RequestType) ListRequestBuilder {
	l.ListFilters.SetRequestType(requestType)
	return l
}

func (l *sqlListRequestsBuilder) WithCorrelationId(correlationId string) ListRequestBuilder {
	l.ListFilters.SetCorrelationId(correlationId)
	return l
}

func (l *sqlListRequestsBuilder) WithConnectionId(u apid.ID) ListRequestBuilder {
	l.ListFilters.SetConnectionId(u)
	return l
}

func (l *sqlListRequestsBuilder) WithConnectorType(t string) ListRequestBuilder {
	l.ListFilters.SetConnectorType(t)
	return l
}

func (l *sqlListRequestsBuilder) WithConnectorId(u apid.ID) ListRequestBuilder {
	l.ListFilters.SetConnectorId(u)
	return l
}

func (l *sqlListRequestsBuilder) WithConnectorVersion(v uint64) ListRequestBuilder {
	l.ListFilters.SetConnectorVersion(v)
	return l
}

func (l *sqlListRequestsBuilder) WithMethod(method string) ListRequestBuilder {
	l.ListFilters.SetMethod(method)
	return l
}

func (l *sqlListRequestsBuilder) WithStatusCode(s int) ListRequestBuilder {
	l.ListFilters.SetStatusCode(s)
	return l
}

func (l *sqlListRequestsBuilder) WithStatusCodeRangeInclusive(start, end int) ListRequestBuilder {
	l.ListFilters.SetStatusCodeRangeInclusive(start, end)
	return l
}

func (l *sqlListRequestsBuilder) WithParsedStatusCodeRange(r string) (ListRequestBuilder, error) {
	start, end, err := util.ParseIntegerRange(r, 100, 999)
	if err != nil {
		return nil, err
	}
	l.ListFilters.SetStatusCodeRangeInclusive(start, end)
	return l, nil
}

func (l *sqlListRequestsBuilder) WithPath(path string) ListRequestBuilder {
	l.ListFilters.SetPath(path)
	return l
}

func (l *sqlListRequestsBuilder) WithPathRegex(r string) (ListRequestBuilder, error) {
	if err := l.ListFilters.SetPathRegex(r); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *sqlListRequestsBuilder) WithTimestampRange(start, end time.Time) ListRequestBuilder {
	l.ListFilters.SetTimestampRange(start, end)
	return l
}

func (l *sqlListRequestsBuilder) WithParsedTimestampRange(r string) (ListRequestBuilder, error) {
	start, end, err := util.ParseTimestampRange(r)
	if err != nil {
		return nil, err
	}
	l.ListFilters.SetTimestampRange(start, end)
	return l, nil
}

func (l *sqlListRequestsBuilder) buildQuery() sq.SelectBuilder {
	builder := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		PlaceholderFormat(l.placeholderFormat)

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

func (l *sqlListRequestsBuilder) fromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	db := l.db
	cursorKey := l.cursorKey
	pf := l.placeholderFormat
	provider := l.provider

	parsed, err := pagination.ParseCursor[sqlListRequestsBuilder](ctx, l.cursorKey, cursor)
	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db
	l.cursorKey = cursorKey
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
		return pagination.PageResult[*LogRecord]{Error: errors.Wrap(err, "failed to build list query")}
	}

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return pagination.PageResult[*LogRecord]{Error: errors.Wrap(err, "failed to execute list query")}
	}
	defer rows.Close()

	entries := make([]*LogRecord, 0)
	for rows.Next() {
		er, err := scanLogRecord(rows)
		if err != nil {
			return pagination.PageResult[*LogRecord]{Error: errors.Wrap(err, "failed to scan entry record")}
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
		cursorStr, err = pagination.MakeCursor(ctx, l.cursorKey, l)
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

func (l *sqlListRequestsBuilder) Enumerate(ctx context.Context, callback func(pagination.PageResult[*LogRecord]) (keepGoing bool, err error)) error {
	var err error
	keepGoing := true
	hasMore := true

	for err == nil && hasMore && keepGoing {
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

// Suppress unused import warnings
var _ = regexp.Compile
