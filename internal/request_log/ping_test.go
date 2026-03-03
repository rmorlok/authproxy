package request_log

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
)

// pingTestPingableStore implements RecordStore and pingable.
type pingTestPingableStore struct {
	pingResult bool
}

func (m *pingTestPingableStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	return nil
}
func (m *pingTestPingableStore) StoreRecords(ctx context.Context, records []*LogRecord) error {
	return nil
}
func (m *pingTestPingableStore) Ping(ctx context.Context) bool {
	return m.pingResult
}

// pingTestNonPingableStore implements RecordStore but not pingable.
type pingTestNonPingableStore struct{}

func (m *pingTestNonPingableStore) StoreRecord(ctx context.Context, record *LogRecord) error {
	return nil
}
func (m *pingTestNonPingableStore) StoreRecords(ctx context.Context, records []*LogRecord) error {
	return nil
}

// pingTestFullStore implements FullStore but not pingable.
type pingTestFullStore struct{}

func (m *pingTestFullStore) Store(ctx context.Context, log *FullLog) error { return nil }
func (m *pingTestFullStore) GetFullLog(ctx context.Context, ns string, id apid.ID) (*FullLog, error) {
	return nil, nil
}

// pingTestPingableFullStore implements FullStore and pingable.
type pingTestPingableFullStore struct {
	pingResult bool
}

func (m *pingTestPingableFullStore) Store(ctx context.Context, log *FullLog) error { return nil }
func (m *pingTestPingableFullStore) GetFullLog(ctx context.Context, ns string, id apid.ID) (*FullLog, error) {
	return nil, nil
}
func (m *pingTestPingableFullStore) Ping(ctx context.Context) bool {
	return m.pingResult
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestStorageServicePing_PingableStoreOk(t *testing.T) {
	ss := &StorageService{
		logger:    testLogger(),
		store:     &pingTestPingableStore{pingResult: true},
		fullStore: &pingTestFullStore{},
	}
	assert.True(t, ss.Ping(context.Background()))
}

func TestStorageServicePing_PingableStoreFails(t *testing.T) {
	ss := &StorageService{
		logger:    testLogger(),
		store:     &pingTestPingableStore{pingResult: false},
		fullStore: &pingTestFullStore{},
	}
	assert.False(t, ss.Ping(context.Background()))
}

func TestStorageServicePing_NonPingableStore(t *testing.T) {
	ss := &StorageService{
		logger:    testLogger(),
		store:     &pingTestNonPingableStore{},
		fullStore: &pingTestFullStore{},
	}
	assert.True(t, ss.Ping(context.Background()))
}

func TestStorageServicePing_PingableFullStoreOk(t *testing.T) {
	ss := &StorageService{
		logger:    testLogger(),
		store:     &pingTestNonPingableStore{},
		fullStore: &pingTestPingableFullStore{pingResult: true},
	}
	assert.True(t, ss.Ping(context.Background()))
}

func TestStorageServicePing_PingableFullStoreFails(t *testing.T) {
	ss := &StorageService{
		logger:    testLogger(),
		store:     &pingTestNonPingableStore{},
		fullStore: &pingTestPingableFullStore{pingResult: false},
	}
	assert.False(t, ss.Ping(context.Background()))
}
