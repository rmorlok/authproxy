package app_metrics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

// recordingStore is a RecordStore that captures each StoreRecords call as a batch. An optional gate blocks writes
// until released, and failWrites makes every write return an error.
type recordingStore struct {
	mu         sync.Mutex
	batches    [][]*LogRecord
	gate       chan struct{}
	failWrites bool
}

func (r *recordingStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	return r.StoreRecords(ctx, []*LogRecord{record})
}

func (r *recordingStore) StoreRecords(_ context.Context, records []*LogRecord) error {
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, append([]*LogRecord(nil), records...))
	if r.failWrites {
		return errors.New("write failed")
	}
	return nil
}

// batchSizes returns the size of each captured batch in write order.
func (r *recordingStore) batchSizes() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	sizes := make([]int, 0, len(r.batches))
	for _, b := range r.batches {
		sizes = append(sizes, len(b))
	}
	return sizes
}

// total returns the number of records written across all batches.
func (r *recordingStore) total() int {
	n := 0
	for _, size := range r.batchSizes() {
		n += size
	}
	return n
}

func newTestRecords(n int) []*LogRecord {
	records := make([]*LogRecord, n)
	for i := range records {
		records[i] = &LogRecord{RequestId: apid.New(apid.PrefixRequestEvents)}
	}
	return records
}

func TestBufferedStore_FlushesFullBatchWithoutWaitingForInterval(t *testing.T) {
	inner := &recordingStore{}
	// A long interval means any flush observed must come from the batch-size trigger.
	store := NewBufferedStore(inner, 10, time.Hour, 100, nil).(*bufferedRecordStore)
	defer store.Close()

	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(10)))

	// The full batch is written as a single insert.
	require.Eventually(t, func() bool { return inner.total() == 10 }, time.Second, 5*time.Millisecond)
	require.Equal(t, []int{10}, inner.batchSizes())
}

func TestBufferedStore_FlushesPartialBatchOnInterval(t *testing.T) {
	inner := &recordingStore{}
	store := NewBufferedStore(inner, 1000, 20*time.Millisecond, 10_000, nil).(*bufferedRecordStore)
	defer store.Close()

	// Fewer records than a batch are still written once the interval elapses.
	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(3)))
	require.Eventually(t, func() bool { return inner.total() == 3 }, time.Second, 5*time.Millisecond)
}

func TestBufferedStore_DropsRecordsBeyondBufferLimit(t *testing.T) {
	// Block the first write so the buffer fills up behind it.
	inner := &recordingStore{gate: make(chan struct{})}
	store := NewBufferedStore(inner, 5, time.Hour, 5, nil).(*bufferedRecordStore)

	// The first five trigger a flush that blocks in the inner store.
	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(5)))
	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.buffer) == 0
	}, time.Second, 5*time.Millisecond)

	// Five more fill the buffer to its limit and three more are dropped without blocking the caller.
	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(8)))
	store.mu.Lock()
	require.Len(t, store.buffer, 5)
	require.Equal(t, 3, store.dropped)
	store.mu.Unlock()

	// Releasing the database lets Close drain everything that was accepted.
	close(inner.gate)
	require.NoError(t, store.Close())
	require.Equal(t, 10, inner.total())
}

func TestBufferedStore_CloseFlushesAndChunksLargeBuffers(t *testing.T) {
	inner := &recordingStore{}
	// A large configured batch size never triggers on its own here, so Close performs the only flush.
	store := NewBufferedStore(inner, 5000, time.Hour, 10_000, nil)

	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(2500)))
	require.NoError(t, store.(*bufferedRecordStore).Close())

	// Writes are split into statements of at most bufferedStoreMaxChunk rows to stay under bind-parameter limits.
	require.Equal(t, []int{1000, 1000, 500}, inner.batchSizes())

	// Closing again is a no-op.
	require.NoError(t, store.(*bufferedRecordStore).Close())
}

func TestBufferedStore_WriteErrorsDoNotStallLaterFlushes(t *testing.T) {
	inner := &recordingStore{failWrites: true}
	store := NewBufferedStore(inner, 2, time.Hour, 100, nil).(*bufferedRecordStore)
	defer store.Close()

	// A failed batch is logged and discarded, and the next batch is still attempted.
	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(2)))
	require.Eventually(t, func() bool { return len(inner.batchSizes()) == 1 }, time.Second, 5*time.Millisecond)
	require.NoError(t, store.StoreRecords(context.Background(), newTestRecords(2)))
	require.Eventually(t, func() bool { return len(inner.batchSizes()) == 2 }, time.Second, 5*time.Millisecond)
}
