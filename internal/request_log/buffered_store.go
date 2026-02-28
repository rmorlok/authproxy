package request_log

import (
	"context"
	"sync"
	"time"
)

type bufferedRecordStore struct {
	inner          RecordStore
	flushBatchSize int
	flushInterval  time.Duration

	// Buffered writer
	mu        sync.Mutex
	buffer    []*LogRecord
	flushCh   chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
}

func NewBufferedStore(inner RecordStore, flushBatchSize int, flushInterval time.Duration) RecordStore {
	s := &bufferedRecordStore{
		inner:          inner,
		flushBatchSize: flushBatchSize,
		flushInterval:  flushInterval,
		buffer:         make([]*LogRecord, 0, flushBatchSize),
		flushCh:        make(chan struct{}, 1),
		closeCh:        make(chan struct{}),
	}

	go s.flushLoop()

	return s
}

func (s *bufferedRecordStore) flushLoop() {
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

func (s *bufferedRecordStore) flush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}
	records := s.buffer
	s.buffer = make([]*LogRecord, 0, s.flushBatchSize)
	s.mu.Unlock()

	ctx := context.Background()
	s.inner.StoreRecords(ctx, records)
}

func (s *bufferedRecordStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	s.mu.Lock()
	s.buffer = append(s.buffer, record)
	shouldFlush := len(s.buffer) >= s.flushBatchSize
	s.mu.Unlock()

	if shouldFlush {
		select {
		case s.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

func (s *bufferedRecordStore) StoreRecords(ctx context.Context, record []*LogRecord) error {
	for _, r := range record {
		if err := s.StoreRecord(ctx, r); err != nil {
			return err
		}
	}

	return nil
}

// TODO: Add support for closing the flusher goroutine in surrounding code
func (s *bufferedRecordStore) Close() error {
	s.closeOnce.Do(func() { close(s.closeCh) })
	return nil
}

var _ RecordStore = (*bufferedRecordStore)(nil)
