package client

import (
	"context"
	"fmt"
	"time"
)

type EncryptionKey struct {
	Id        string            `json:"id"`
	Namespace string            `json:"namespace"`
	State     string            `json:"state"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type CreateEncryptionKeyRequest struct {
	Namespace string                 `json:"namespace"`
	Labels    map[string]string      `json:"labels,omitempty"`
	KeyData   map[string]interface{} `json:"key_data"`
}

type UpdateEncryptionKeyRequest struct {
	State  *string            `json:"state,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
}

func (c *Client) CreateEncryptionKey(ctx context.Context, req CreateEncryptionKeyRequest) (*EncryptionKey, error) {
	var ek EncryptionKey
	err := c.post(ctx, "/api/v1/encryption-keys", req, &ek)
	return &ek, err
}

func (c *Client) GetEncryptionKey(ctx context.Context, id string) (*EncryptionKey, error) {
	var ek EncryptionKey
	err := c.get(ctx, fmt.Sprintf("/api/v1/encryption-keys/%s", id), &ek)
	return &ek, err
}

func (c *Client) UpdateEncryptionKey(ctx context.Context, id string, req UpdateEncryptionKeyRequest) (*EncryptionKey, error) {
	var ek EncryptionKey
	err := c.patch(ctx, fmt.Sprintf("/api/v1/encryption-keys/%s", id), req, &ek)
	return &ek, err
}

func (c *Client) DeleteEncryptionKey(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/encryption-keys/%s", id))
}
