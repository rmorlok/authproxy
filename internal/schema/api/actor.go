package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
)

// ListActorsResponseJson is the Kubernetes-style Actor list response.
type ListActorsResponseJson struct {
	apiv1alpha1.ResourceList[actorschema.Actor] `json:",inline" yaml:",inline"`
}

func NewListActorsResponseJson(items []actorschema.Actor, continueToken string) ListActorsResponseJson {
	return ListActorsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			actorschema.ActorKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}
