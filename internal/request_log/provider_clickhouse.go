package request_log

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// --- ClickHouse EntryRecordStore ---

type clickhouseEntryRecordStore struct {
	db     *sql.DB
	cfg    *config.DatabaseClickhouse
	logger *slog.Logger

	// Buffered writer
	mu        sync.Mutex
	buffer    []*EntryRecord
	flushCh   chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
}

func NewClickhouseEntryRecordStore(cfg config.Database, logger *slog.Logger) EntryRecordStore {
	chCfg, ok := cfg.InnerVal.(*config.DatabaseClickhouse)
	if !ok {
		panic(fmt.Sprintf("expected *config.DatabaseClickhouse, got %T", cfg))
	}

	opts, err := chCfg.ToClickhouseOptions()
	if err != nil {
		panic(errors.Wrap(err, "failed to convert clickhouse config to options"))
	}

	db := clickhouse.OpenDB(opts)
	s := &clickhouseEntryRecordStore{
		db:      db,
		cfg:     chCfg,
		logger:  logger,
		buffer:  make([]*EntryRecord, 0, chCfg.GetFlushBatchSize()),
		flushCh: make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}

	go s.flushLoop()

	return s
}

func (s *clickhouseEntryRecordStore) flushLoop() {
	ticker := time.NewTicker(s.cfg.GetFlushInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.flushCh:
			s.flush()
		case <-s.closeCh:
			s.flush()
			return
		}
	}
}

func (s *clickhouseEntryRecordStore) flush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}
	records := s.buffer
	s.buffer = make([]*EntryRecord, 0, s.cfg.GetFlushBatchSize())
	s.mu.Unlock()

	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("failed to begin clickhouse transaction", "error", err)
		return
	}

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (request_id, namespace, type, correlation_id, timestamp_ms, "+
			"duration_ms, connection_id, connector_id, connector_version, "+
			"method, host, scheme, path, "+
			"response_status_code, response_error, "+
			"request_http_version, request_size_bytes, request_mime_type, "+
			"response_http_version, response_size_bytes, response_mime_type, "+
			"internal_timeout, request_cancelled, full_request_recorded) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		entryRecordsTable,
	))
	if err != nil {
		s.logger.Error("failed to prepare clickhouse insert", "error", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, r := range records {
		_, err = stmt.ExecContext(ctx,
			r.RequestId.String(), r.Namespace, string(r.Type), r.CorrelationId,
			r.Timestamp.UnixMilli(), r.MillisecondDuration.Duration().Milliseconds(),
			r.ConnectionId.String(), r.ConnectorId.String(), r.ConnectorVersion,
			r.Method, r.Host, r.Scheme, r.Path,
			r.ResponseStatusCode, r.ResponseError,
			r.RequestHttpVersion, r.RequestSizeBytes, r.RequestMimeType,
			r.ResponseHttpVersion, r.ResponseSizeBytes, r.ResponseMimeType,
			r.InternalTimeout, r.RequestCancelled, r.FullRequestRecorded,
		)
		if err != nil {
			s.logger.Error("failed to insert record into clickhouse", "error", err, "entry_id", r.RequestId.String())
		}
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit clickhouse transaction", "error", err)
	}
}

func (s *clickhouseEntryRecordStore) StoreRecord(ctx context.Context, record *EntryRecord) error {
	s.mu.Lock()
	s.buffer = append(s.buffer, record)
	shouldFlush := len(s.buffer) >= s.cfg.GetFlushBatchSize()
	s.mu.Unlock()

	if shouldFlush {
		select {
		case s.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

func (s *clickhouseEntryRecordStore) Migrate(ctx context.Context) error {
	s.logger.Info("running clickhouse http log migrations")

	ddl := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		request_id String,
		namespace String,
		type String,
		correlation_id String,
		timestamp_ms Int64,
		duration_ms Int64,
		connection_id String,
		connector_id String,
		connector_version UInt64,
		method String,
		host String,
		scheme String,
		path String,
		response_status_code Int32,
		response_error String,
		request_http_version String,
		request_size_bytes Int64,
		request_mime_type String,
		response_http_version String,
		response_size_bytes Int64,
		response_mime_type String,
		internal_timeout Bool,
		request_cancelled Bool,
		full_request_recorded Bool
	) ENGINE = MergeTree()
	ORDER BY (namespace, timestamp_ms, request_id)`, entryRecordsTable)

	_, err := s.db.ExecContext(ctx, ddl)
	if err != nil {
		return errors.Wrap(err, "failed to create clickhouse table")
	}

	s.logger.Info("clickhouse http log migrations complete")
	return nil
}

var _ EntryRecordStore = (*clickhouseEntryRecordStore)(nil)

// --- ClickHouse EntryRecordRetriever ---

type clickhouseEntryRecordRetriever struct {
	db        *sql.DB
	cfg       *config.HttpLoggingDatabaseClickhouse
	cursorKey config.KeyDataType
	logger    *slog.Logger
}

func NewClickhouseEntryRecordRetriever(cfg config.HttpLoggingDatabase, cursorKey config.KeyDataType, logger *slog.Logger) EntryRecordRetriever {
	chCfg, ok := cfg.(*config.HttpLoggingDatabaseClickhouse)
	if !ok {
		panic(fmt.Sprintf("expected *config.HttpLoggingDatabaseClickhouse, got %T", cfg))
	}

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: chCfg.Addresses,
		Auth: clickhouse.Auth{
			Database: chCfg.Database,
		},
	})

	return &clickhouseEntryRecordRetriever{
		db:        db,
		cfg:       chCfg,
		cursorKey: cursorKey,
		logger:    logger,
	}
}

func (r *clickhouseEntryRecordRetriever) GetRecord(ctx context.Context, id uuid.UUID) (*EntryRecord, error) {
	query, args, err := sq.Select(entryRecordColumns...).
		From(entryRecordsTable).
		Where(sq.Eq{"request_id": id.String()}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build select query")
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	er, err := scanEntryRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to get entry record from clickhouse")
	}

	return er, nil
}

func (r *clickhouseEntryRecordRetriever) NewListRequestsBuilder() ListRequestBuilder {
	return &clickhouseListRequestsBuilder{
		sqlListRequestsBuilder: sqlListRequestsBuilder{
			ListFilters:       ListFilters{LimitVal: 100},
			db:                r.db,
			cursorKey:         r.cursorKey,
			placeholderFormat: sq.Question,
			provider:          config.HttpLoggingDatabaseProviderClickhouse,
		},
	}
}

func (r *clickhouseEntryRecordRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	b := &clickhouseListRequestsBuilder{
		sqlListRequestsBuilder: sqlListRequestsBuilder{
			ListFilters:       ListFilters{LimitVal: 100},
			db:                r.db,
			cursorKey:         r.cursorKey,
			placeholderFormat: sq.Question,
			provider:          config.HttpLoggingDatabaseProviderClickhouse,
		},
	}

	return b.fromCursor(ctx, cursor)
}

var _ EntryRecordRetriever = (*clickhouseEntryRecordRetriever)(nil)

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
	cursorKey := l.cursorKey
	pf := l.placeholderFormat
	provider := l.provider

	parsed, err := pagination.ParseCursor[clickhouseListRequestsBuilder](ctx, l.cursorKey, cursor)
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

// Forward all ListRequestBuilder methods to ensure correct return types

func (l *clickhouseListRequestsBuilder) Limit(limit int32) ListRequestBuilder {
	l.sqlListRequestsBuilder.Limit(limit)
	return l
}

func (l *clickhouseListRequestsBuilder) OrderBy(field RequestOrderByField, by pagination.OrderBy) ListRequestBuilder {
	l.sqlListRequestsBuilder.OrderBy(field, by)
	return l
}

func (l *clickhouseListRequestsBuilder) WithNamespaceMatcher(matcher string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithNamespaceMatcher(matcher)
	return l
}

func (l *clickhouseListRequestsBuilder) WithNamespaceMatchers(matchers []string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithNamespaceMatchers(matchers)
	return l
}

func (l *clickhouseListRequestsBuilder) WithRequestType(requestType RequestType) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithRequestType(requestType)
	return l
}

func (l *clickhouseListRequestsBuilder) WithCorrelationId(correlationId string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithCorrelationId(correlationId)
	return l
}

func (l *clickhouseListRequestsBuilder) WithConnectionId(u uuid.UUID) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithConnectionId(u)
	return l
}

func (l *clickhouseListRequestsBuilder) WithConnectorType(t string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithConnectorType(t)
	return l
}

func (l *clickhouseListRequestsBuilder) WithConnectorId(u uuid.UUID) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithConnectorId(u)
	return l
}

func (l *clickhouseListRequestsBuilder) WithConnectorVersion(v uint64) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithConnectorVersion(v)
	return l
}

func (l *clickhouseListRequestsBuilder) WithMethod(method string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithMethod(method)
	return l
}

func (l *clickhouseListRequestsBuilder) WithStatusCode(s int) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithStatusCode(s)
	return l
}

func (l *clickhouseListRequestsBuilder) WithStatusCodeRangeInclusive(start, end int) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithStatusCodeRangeInclusive(start, end)
	return l
}

func (l *clickhouseListRequestsBuilder) WithParsedStatusCodeRange(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.WithParsedStatusCodeRange(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) WithPath(path string) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithPath(path)
	return l
}

func (l *clickhouseListRequestsBuilder) WithPathRegex(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.WithPathRegex(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *clickhouseListRequestsBuilder) WithTimestampRange(start, end time.Time) ListRequestBuilder {
	l.sqlListRequestsBuilder.WithTimestampRange(start, end)
	return l
}

func (l *clickhouseListRequestsBuilder) WithParsedTimestampRange(r string) (ListRequestBuilder, error) {
	_, err := l.sqlListRequestsBuilder.WithParsedTimestampRange(r)
	if err != nil {
		return nil, err
	}
	return l, nil
}

var _ ListRequestExecutor = (*clickhouseListRequestsBuilder)(nil)
var _ ListRequestBuilder = (*clickhouseListRequestsBuilder)(nil)

// Suppress unused import warnings
var _ = strings.Contains
