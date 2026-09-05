package client

import (
	"context"
	"fmt"
	"time"
)

const (
	ActorAPIVersion = "authproxy.net/v1alpha1"
	ActorKind       = "Actor"
)

// These local wire models keep the Terraform provider independent from the
// server's internal packages while mirroring its canonical resource contract.
type ActorMetadata struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty"`
}

type ActorMetadataPatch struct {
	Name        *string            `json:"name,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

type ActorSpec struct {
	ExternalID string `json:"externalId"`
}

type ActorSpecPatch struct{}

type Actor struct {
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Metadata   ActorMetadata `json:"metadata"`
	Spec       ActorSpec     `json:"spec"`
}

type CreateActorRequest struct {
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Metadata   ActorMetadata `json:"metadata"`
	Spec       ActorSpec     `json:"spec"`
}

type UpdateActorRequest struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   *ActorMetadataPatch `json:"metadata"`
	Spec       *ActorSpecPatch     `json:"spec"`
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
