package client

import (
	"context"
	"fmt"
	"time"
)

type Key struct {
	Id          string            `json:"id"`
	Namespace   string            `json:"namespace"`
	State       string            `json:"state"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type CreateKeyRequest struct {
	Namespace   string                 `json:"namespace"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	KeyData     map[string]interface{} `json:"key_data"`
}

type UpdateKeyRequest struct {
	State       *string            `json:"state,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

func (c *Client) CreateKey(ctx context.Context, req CreateKeyRequest) (*Key, error) {
	var ek Key
	err := c.post(ctx, "/api/v1/keys", req, &ek)
	return &ek, err
}

func (c *Client) GetKey(ctx context.Context, id string) (*Key, error) {
	var ek Key
	err := c.get(ctx, fmt.Sprintf("/api/v1/keys/%s", id), &ek)
	return &ek, err
}

func (c *Client) UpdateKey(ctx context.Context, id string, req UpdateKeyRequest) (*Key, error) {
	var ek Key
	err := c.patch(ctx, fmt.Sprintf("/api/v1/keys/%s", id), req, &ek)
	return &ek, err
}

func (c *Client) DeleteKey(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/keys/%s", id))
}
