package core

import (
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type IActorData interface {
	GetId() apid.ID
	GetExternalId() string
	GetPermissions() []aschema.Permission
	GetNamespace() string
	GetLabels() map[string]string
	GetAnnotations() map[string]string
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
	Annotations map[string]string    `json:"annotations,omitempty"`
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

// GetAnnotations returns actor annotations from database state or JWT claims.
func (a *Actor) GetAnnotations() map[string]string { return a.Annotations }

func isValidValueForNamespaceTemplating(val string) bool {
	return val != "" && val != namespace.NamespaceWildcard
}

func filterLabelOrAnnotationForPermission(vals map[string]string) map[string]string {
	result := make(map[string]string, len(vals))

	for k, v := range vals {
		if isValidValueForNamespaceTemplating(v) {
			result[k] = v
		}
	}

	return result
}

// GetPermissionTemplateData returns actor data for permission template context. These are
// the values that can be used in templated namespaces for permissions baed on actor data.
// E.g. root.{{labels.team_id}}
func (a *Actor) GetPermissionTemplateData() map[string]any {
	result := map[string]any{
		"labels":      filterLabelOrAnnotationForPermission(a.Labels),
		"annotations": filterLabelOrAnnotationForPermission(a.Annotations),
	}

	if isValidValueForNamespaceTemplating(a.ExternalId) {
		result["external_id"] = a.ExternalId
	}

	return result
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
		Annotations: data.GetAnnotations(),
		Permissions: data.GetPermissions(),
	}
}

var _ IActorData = &Actor{}
