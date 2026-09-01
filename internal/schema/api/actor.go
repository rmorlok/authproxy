package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// ActorJson represents an actor returned by the API.
//
//	@Description	Actor identity within a namespace
type ActorJson struct {
	Id          apid.ID             `json:"id" yaml:"id" swaggertype:"string" example:"act_test550e8400abcde"`
	Namespace   string              `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName `json:"name" yaml:"name" swaggertype:"string" example:"billing-service"`
	ExternalId  string              `json:"externalId" yaml:"externalId" example:"user-123"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time           `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt" yaml:"updatedAt"`
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
	Labels      map[string]string    `json:"labels" yaml:"labels"`
	Annotations map[string]string    `json:"annotations" yaml:"annotations"`
}

// ListActorsResponseJson is the Kubernetes-style actor list response.
type ListActorsResponseJson struct {
	apiv1alpha1.ResourceList[ActorJson] `json:",inline" yaml:",inline"`
}

func NewListActorsResponseJson(items []ActorJson, continueToken string) ListActorsResponseJson {
	return ListActorsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			"Actor",
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}
