package core

import (
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type IActorData interface {
	GetId() apid.ID
	GetExternalId() string
	GetPermissions() []aschema.Permission
	GetNamespace() string
	GetLabels() map[string]string
}

// Actor is the information that identifies who is making a request. This can be an actor in the calling
// system, an admin from the calling system, a devops admin from the cli, etc.
type Actor struct {
	// This version of the actor is deserialized from the JWT directly. The JSON annotations apply to
	// how the JWT is structured.

	Id          apid.ID              `json:"-"` // This is the database ID of the actor. It cannot be set in the JWT directly.
	ExternalId  string               `json:"external_id"`
	Namespace   string               `json:"namespace,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty"`
	Permissions []aschema.Permission `json:"permissions"`
}

func (a *Actor) GetId() apid.ID {
	return a.Id
}

func (a *Actor) GetExternalId() string {
	return a.ExternalId
}

func (a *Actor) GetPermissions() []aschema.Permission {
	return a.Permissions
}

func (a *Actor) GetNamespace() string {
	return a.Namespace
}

func (a *Actor) GetLabels() map[string]string { return a.Labels }

func CreateActor(data IActorData) *Actor {
	if a, ok := data.(*Actor); ok {
		return a
	}

	return &Actor{
		Id:          data.GetId(),
		ExternalId:  data.GetExternalId(),
		Namespace:   data.GetNamespace(),
		Labels:      data.GetLabels(),
		Permissions: data.GetPermissions(),
	}
}

var _ IActorData = &Actor{}
