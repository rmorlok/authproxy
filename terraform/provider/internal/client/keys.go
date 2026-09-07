package client

import (
	"context"
	"fmt"
)

const KeyKind = "Key"

type KeySpec struct {
	Usage        string         `json:"usage,omitempty"`
	MaterialType string         `json:"materialType,omitempty"`
	DesiredState string         `json:"desiredState,omitempty"`
	KeyData      map[string]any `json:"keyData,omitempty"`
}

type KeyStatus struct {
	State             string `json:"state"`
	KeyDataConfigured bool   `json:"keyDataConfigured"`
}

type Key struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     KeySpec        `json:"spec"`
	Status   *KeyStatus     `json:"status,omitempty"`
}

type CreateKeyRequest struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     KeySpec        `json:"spec"`
}

type KeySpecPatch struct {
	DesiredState *string `json:"desiredState,omitempty"`
}

type UpdateKeyRequest struct {
	TypeMeta
	Metadata *ObjectMetadataPatch `json:"metadata"`
	Spec     *KeySpecPatch        `json:"spec"`
}

func (c *Client) CreateKey(ctx context.Context, req CreateKeyRequest) (*Key, error) {
	var key Key
	err := c.post(ctx, "/api/v1/keys", req, &key)
	return &key, err
}

func (c *Client) GetKey(ctx context.Context, id string) (*Key, error) {
	var key Key
	err := c.get(ctx, fmt.Sprintf("/api/v1/keys/%s", id), &key)
	return &key, err
}

func (c *Client) UpdateKey(ctx context.Context, id string, req UpdateKeyRequest) (*Key, error) {
	var key Key
	err := c.patch(ctx, fmt.Sprintf("/api/v1/keys/%s", id), req, &key)
	return &key, err
}

func (c *Client) DeleteKey(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/keys/%s", id))
}
