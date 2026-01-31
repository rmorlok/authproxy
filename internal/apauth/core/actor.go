package core

import (
	"github.com/google/uuid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type IActorData interface {
	GetId() uuid.UUID
	GetExternalId() string
	GetPermissions() []aschema.Permission
	IsAdmin() bool
	IsSuperAdmin() bool
	GetEmail() string
	GetNamespace() string
	GetLabels() map[string]string
}

// Actor is the information that identifies who is making a request. This can be an actor in the calling
// system, an admin from the calling system, a devops admin from the cli, etc.
type Actor struct {
	// This version of the actor is deserialized from the JWT directly. The JSON annotations apply to
	// how the JWT is structured.

	Id          uuid.UUID            `json:"-"` // This is the database ID of the actor. It cannot be set in the JWT directly.
	ExternalId  string               `json:"external_id"`
	Namespace   string               `json:"namespace,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty"`
	Permissions []aschema.Permission `json:"permissions"`
	Admin       bool                 `json:"admin,omitempty"`
	SuperAdmin  bool                 `json:"super_admin,omitempty"`
	Email       string               `json:"email,omitempty"`
}

func (a *Actor) GetId() uuid.UUID {
	return a.Id
}

func (a *Actor) GetExternalId() string {
	return a.ExternalId
}

func (a *Actor) GetPermissions() []aschema.Permission {
	return a.Permissions
}

func (a *Actor) GetEmail() string {
	return a.Email
}

func (a *Actor) GetNamespace() string {
	return a.Namespace
}

func (a *Actor) GetLabels() map[string]string { return a.Labels }

// IsAdmin is a helper to wrap the Admin attribute
func (a *Actor) IsAdmin() bool {
	if a == nil {
		return false
	}

	return a.Admin
}

// IsSuperAdmin is a helper to wrap the SuperAdmin attribute
func (a *Actor) IsSuperAdmin() bool {
	if a == nil {
		return false
	}

	return a.SuperAdmin
}

// IsNormalActor indicates that an actor is not an admin or superadmin
func (a *Actor) IsNormalActor() bool {
	if a == nil {
		// actors default to normal
		return true
	}

	return !a.IsSuperAdmin() && !a.IsAdmin()
}

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
		Admin:       data.IsAdmin(),
		SuperAdmin:  data.IsSuperAdmin(),
		Email:       data.GetEmail(),
	}
}

var _ IActorData = &Actor{}
