package apblob

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// NewFromConfig creates a new blob storage client from the specified configuration. The type of the client
// returned will be determined by the configuration. If cfg is nil, returns an in-memory client.
func NewFromConfig(ctx context.Context, cfg *config.BlobStorage) (Client, error) {
	if cfg == nil || cfg.InnerVal == nil {
		return NewMemoryClient(), nil
	}

	switch v := cfg.InnerVal.(type) {
	case *config.BlobStorageMemory:
		return NewMemoryClient(), nil
	case *config.BlobStorageS3:
		return NewS3Client(ctx, v)
	default:
		return nil, errors.New("blob storage type not supported")
	}
}
