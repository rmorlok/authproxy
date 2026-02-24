package apblob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/schema/config"
)

func TestNewFromConfig_NilConfig(t *testing.T) {
	client, err := NewFromConfig(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Should be a usable memory client
	require.NoError(t, client.Put(context.Background(), PutInput{Key: "k", Data: []byte("v")}))
	data, err := client.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("v"), data)
}

func TestNewFromConfig_NilInnerVal(t *testing.T) {
	client, err := NewFromConfig(context.Background(), &config.BlobStorage{InnerVal: nil})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewFromConfig_MemoryProvider(t *testing.T) {
	cfg := &config.BlobStorage{
		InnerVal: &config.BlobStorageMemory{
			Provider: config.BlobStorageProviderMemory,
		},
	}

	client, err := NewFromConfig(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	_, ok := client.(*MemoryClient)
	assert.True(t, ok, "memory provider should return *MemoryClient")
}
