package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// ActorJson represents an actor returned by the API.
//
//	@Description	Actor identity within a namespace
type ActorJson struct {
	Id          apid.ID              `json:"id" yaml:"id" swaggertype:"string" example:"act_test550e8400abcde"`
	Namespace   string               `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName  `json:"name" yaml:"name" swaggertype:"string" example:"billing-service"`
	ExternalId  string               `json:"externalId" yaml:"externalId" example:"user-123"`
	Permissions []aschema.Permission `json:"permissions" yaml:"permissions"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time            `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt" yaml:"updatedAt"`
}

// CreateActorRequestJson represents a request to create an actor.
//
//	@Description	Actor creation request
type CreateActorRequestJson struct {
	ExternalId  string               `json:"externalId" yaml:"externalId" example:"user-123"`
	Namespace   string               `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"billing-service"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateActorRequestJson represents a request to update actor metadata.
//
//	@Description	Actor update request
type UpdateActorRequestJson struct {
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"billing-service"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Labels      map[string]string    `json:"labels" yaml:"labels"`
	Annotations map[string]string    `json:"annotations" yaml:"annotations"`
}

// ListActorsResponseJson is a paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	Items  []ActorJson `json:"items" yaml:"items"`
	Cursor string      `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}
