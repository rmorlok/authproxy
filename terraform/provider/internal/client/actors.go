package client

import (
	"context"
	"fmt"
	"time"
)

type Actor struct {
	Id          string            `json:"id"`
	Namespace   string            `json:"namespace"`
	ExternalId  string            `json:"external_id"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type CreateActorRequest struct {
	ExternalId  string            `json:"external_id"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type UpdateActorRequest struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func (c *Client) CreateActor(ctx context.Context, req CreateActorRequest) (*Actor, error) {
	var a Actor
	err := c.post(ctx, "/api/v1/actors", req, &a)
	return &a, err
}

func (c *Client) GetActor(ctx context.Context, id string) (*Actor, error) {
	var a Actor
	err := c.get(ctx, fmt.Sprintf("/api/v1/actors/%s", id), &a)
	return &a, err
}

func (c *Client) UpdateActor(ctx context.Context, id string, req UpdateActorRequest) (*Actor, error) {
	var a Actor
	err := c.patch(ctx, fmt.Sprintf("/api/v1/actors/%s", id), req, &a)
	return &a, err
}

func (c *Client) DeleteActor(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/actors/%s", id))
}
