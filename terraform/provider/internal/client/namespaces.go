package client

import (
	"context"
	"fmt"
	"time"
)

type Namespace struct {
	Path            string            `json:"path"`
	State           string            `json:"state"`
	EncryptionKeyId *string           `json:"encryption_key_id,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type CreateNamespaceRequest struct {
	Path   string            `json:"path"`
	Labels map[string]string `json:"labels,omitempty"`
}

type UpdateNamespaceRequest struct {
	Labels map[string]string `json:"labels,omitempty"`
}

func (c *Client) CreateNamespace(ctx context.Context, req CreateNamespaceRequest) (*Namespace, error) {
	var ns Namespace
	err := c.post(ctx, "/api/v1/namespaces", req, &ns)
	return &ns, err
}

func (c *Client) GetNamespace(ctx context.Context, path string) (*Namespace, error) {
	var ns Namespace
	err := c.get(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path), &ns)
	return &ns, err
}

func (c *Client) UpdateNamespace(ctx context.Context, path string, req UpdateNamespaceRequest) (*Namespace, error) {
	var ns Namespace
	err := c.patch(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path), req, &ns)
	return &ns, err
}

func (c *Client) DeleteNamespace(ctx context.Context, path string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path))
}
