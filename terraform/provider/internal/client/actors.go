package client

import (
	"context"
	"fmt"
)

const ActorKind = "Actor"

// These local wire models keep the Terraform provider independent from the
// server's internal packages while mirroring its canonical resource contract.
type ActorSpec struct {
	ExternalID string `json:"externalId"`
}

type ActorSpecPatch struct{}

type Actor struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     ActorSpec      `json:"spec"`
}

type CreateActorRequest struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     ActorSpec      `json:"spec"`
}

type UpdateActorRequest struct {
	TypeMeta
	Metadata *ObjectMetadataPatch `json:"metadata"`
	Spec     *ActorSpecPatch      `json:"spec"`
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
