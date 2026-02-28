package apblob

import (
	"context"
	"sync"
)

// MemoryClient is an in-memory blob storage client for testing.
type MemoryClient struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemoryClient creates an in-memory blob storage client for testing.
func NewMemoryClient() *MemoryClient {
	return &MemoryClient{
		data: make(map[string][]byte),
	}
}

func (m *MemoryClient) Put(_ context.Context, input PutInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(input.Data))
	copy(cp, input.Data)
	m.data[input.Key] = cp
	return nil
}

func (m *MemoryClient) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.data[key]
	if !ok {
		return nil, ErrBlobNotFound
	}
	cp := make([]byte, len(d))
	copy(cp, d)
	return cp, nil
}

func (m *MemoryClient) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// Keys returns all keys currently stored (useful for testing).
func (m *MemoryClient) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

var _ Client = (*MemoryClient)(nil)
