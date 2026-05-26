package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
)

// ActorJson represents an actor returned by the API.
//
//	@Description	Actor identity within a namespace
type ActorJson struct {
	Id          apid.ID           `json:"id" yaml:"id" swaggertype:"string" example:"act_test550e8400abcde"`
	Namespace   string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	ExternalId  string            `json:"external_id" yaml:"external_id" example:"user-123"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// CreateActorRequestJson represents a request to create an actor.
//
//	@Description	Actor creation request
type CreateActorRequestJson struct {
	ExternalId  string            `json:"external_id" yaml:"external_id" example:"user-123"`
	Namespace   string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateActorRequestJson represents a request to update actor metadata.
//
//	@Description	Actor update request
type UpdateActorRequestJson struct {
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

// ListActorsResponseJson is a paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	Items  []ActorJson `json:"items" yaml:"items"`
	Cursor string      `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}
