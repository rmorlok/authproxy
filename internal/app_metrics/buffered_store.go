package app_metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// bufferedStoreWriteTimeout bounds each batch write so a hung database cannot stall the flush loop indefinitely.
	bufferedStoreWriteTimeout = 30 * time.Second

	// bufferedStoreMaxChunk caps the number of rows in one INSERT. Request-event inserts bind ~32 parameters per row
	// and Postgres allows at most 65535 bind parameters per statement, so larger configured batch sizes are written
	// as several statements.
	bufferedStoreMaxChunk = 1000
)

// bufferedRecordStore batches request-event records in memory and writes them to the inner store in bulk.
//
// The proxy records one event per proxied request, so writing each one individually would mean one INSERT per
// request; that does not scale on Postgres and is pathological on ClickHouse, where every insert creates a new part.
// Records are flushed when the buffer reaches flushBatchSize or every flushInterval, whichever comes first.
//
// The buffer is bounded by maxBuffered. When the database cannot keep up, new records are dropped (and counted) rather
// than letting memory grow without limit; request metrics are best-effort telemetry and must never take down the
// proxy. Records still buffered when the process exits uncleanly are lost; Close flushes them on graceful shutdown.
type bufferedRecordStore struct {
	inner          RecordStore
	logger         *slog.Logger
	flushBatchSize int
	flushInterval  time.Duration
	maxBuffered    int

	// mu guards buffer and dropped.
	mu     sync.Mutex
	buffer []*LogRecord
	// dropped counts records discarded because the buffer was full since the last report.
	dropped int

	flushCh   chan struct{}
	closeCh   chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

// NewBufferedStore wraps inner so that StoreRecord/StoreRecords enqueue records and return immediately; a background
// goroutine writes them in batches. flushBatchSize, flushInterval, and maxBuffered fall back to safe defaults when
// non-positive. The caller must Close the returned store to flush pending records and stop the goroutine.
func NewBufferedStore(inner RecordStore, flushBatchSize int, flushInterval time.Duration, maxBuffered int, logger *slog.Logger) RecordStore {
	if flushBatchSize <= 0 {
		flushBatchSize = 1000
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	if maxBuffered < flushBatchSize {
		maxBuffered = flushBatchSize
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &bufferedRecordStore{
		inner:          inner,
		logger:         logger,
		flushBatchSize: flushBatchSize,
		flushInterval:  flushInterval,
		maxBuffered:    maxBuffered,
		buffer:         make([]*LogRecord, 0, flushBatchSize),
		flushCh:        make(chan struct{}, 1),
		closeCh:        make(chan struct{}),
		doneCh:         make(chan struct{}),
	}

	go s.flushLoop()

	return s
}

// flushLoop runs until Close, flushing on the interval ticker or when a full batch is signalled. Flushes are
// serialized on this goroutine, so at most one batch write is in flight at a time.
func (s *bufferedRecordStore) flushLoop() {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.flushInterval)
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

// flush swaps out the current buffer and writes it to the inner store in chunks. Write errors are logged and the
// failed records are discarded; retrying would let a persistently failing database pin the buffer at its limit.
func (s *bufferedRecordStore) flush() {
	s.mu.Lock()
	records := s.buffer
	dropped := s.dropped
	s.buffer = make([]*LogRecord, 0, s.flushBatchSize)
	s.dropped = 0
	s.mu.Unlock()

	if dropped > 0 {
		s.logger.Warn("dropped request event records because the write buffer was full",
			"dropped", dropped, "max_buffered", s.maxBuffered)
	}

	for start := 0; start < len(records); start += bufferedStoreMaxChunk {
		end := min(start+bufferedStoreMaxChunk, len(records))
		chunk := records[start:end]

		ctx, cancel := context.WithTimeout(context.Background(), bufferedStoreWriteTimeout)
		err := s.inner.StoreRecords(ctx, chunk)
		cancel()
		if err != nil {
			s.logger.Error("failed to write buffered request event records", "error", err, "records", len(chunk))
		}
	}
}

// StoreRecord enqueues a record for the next flush. It never blocks on the database and always returns nil; if the
// buffer is full the record is dropped and counted.
func (s *bufferedRecordStore) StoreRecord(_ context.Context, record *LogRecord) error {
	s.mu.Lock()
	if len(s.buffer) >= s.maxBuffered {
		s.dropped++
		s.mu.Unlock()
		return nil
	}
	s.buffer = append(s.buffer, record)
	shouldFlush := len(s.buffer) >= s.flushBatchSize
	s.mu.Unlock()

	if shouldFlush {
		// Non-blocking: a pending signal already guarantees a flush.
		select {
		case s.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// StoreRecords enqueues each record; see StoreRecord.
func (s *bufferedRecordStore) StoreRecords(ctx context.Context, records []*LogRecord) error {
	for _, r := range records {
		if err := s.StoreRecord(ctx, r); err != nil {
			return err
		}
	}

	return nil
}

// Ping reports whether the inner store is reachable, so health checks see through the buffer.
func (s *bufferedRecordStore) Ping(ctx context.Context) bool {
	if p, ok := s.inner.(pingable); ok {
		return p.Ping(ctx)
	}
	return true
}

// Close stops accepting background flushes, writes any buffered records, and waits for the flush goroutine to exit.
// It is safe to call more than once.
func (s *bufferedRecordStore) Close() error {
	s.closeOnce.Do(func() { close(s.closeCh) })
	<-s.doneCh
	return nil
}

var _ RecordStore = (*bufferedRecordStore)(nil)
