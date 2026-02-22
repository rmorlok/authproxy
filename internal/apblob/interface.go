package apblob

import (
	"context"
	"time"
)

type PutInput struct {
	Key         string
	Data        []byte
	ContentType *string
	ExpiresAt   *time.Time
}

// Client is the interface for blob storage operations.
type Client interface {
	// Put stores data under the given key with the specified content type.
	Put(ctx context.Context, input PutInput) error

	// Get retrieves data stored under the given key.
	// Returns ErrBlobNotFound if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes data stored under the given key.
	Delete(ctx context.Context, key string) error
}
