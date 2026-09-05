package core

import (
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// Actor is the core abstraction around a persisted actor identity.
type Actor struct {
	database.Actor

	s      *service
	logger *slog.Logger
}

func wrapActor(actor database.Actor, s *service) *Actor {
	return &Actor{
		Actor: actor,
		s:     s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(actor.Namespace).
			Build(),
	}
}

// actorResourceFromDatabase converts the flat persistence model into the
// canonical resource. Encrypted signing material is deliberately represented
// only by status and is never decrypted at this boundary.
func actorResourceFromDatabase(actor database.Actor) *actorschema.Actor {
	createdAt := actor.CreatedAt
	updatedAt := actor.UpdatedAt
	return &actorschema.Actor{
		TypeMeta: meta.NewTypeMeta(actorschema.ActorKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          actor.Id.String(),
			Name:        actor.Name,
			Namespace:   actor.Namespace,
			Labels:      maps.Clone(map[string]string(actor.Labels)),
			Annotations: maps.Clone(map[string]string(actor.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: actorschema.ActorSpec{
			ExternalId:  actor.ExternalId,
			Permissions: actorschema.ClonePermissions([]authschema.Permission(actor.Permissions)),
		},
		Status: &actorschema.ActorStatus{
			SigningKeyConfigured: actor.CanSelfSign(),
		},
	}
}

func databaseActorFromResource(
	resource *actorschema.Actor,
	id apid.ID,
	encryptedKey *encfield.EncryptedField,
) (*database.Actor, error) {
	if resource == nil {
		return nil, fmt.Errorf("actor is required")
	}
	userLabels, _ := database.SplitUserAndApxyLabels(database.Labels(resource.Metadata.Labels))
	return &database.Actor{
		Id:           id,
		Namespace:    resource.Metadata.Namespace,
		Name:         resource.Metadata.Name,
		ExternalId:   resource.Spec.ExternalId,
		Permissions:  database.Permissions(actorschema.ClonePermissions(resource.Spec.Permissions)),
		Labels:       userLabels,
		Annotations:  database.Annotations(maps.Clone(resource.Metadata.Annotations)),
		EncryptedKey: encryptedKey,
	}, nil
}

func (a *Actor) GetId() apid.ID                          { return a.Id }
func (a *Actor) GetNamespace() string                    { return a.Namespace }
func (a *Actor) GetName() common.ResourceName            { return a.Name }
func (a *Actor) GetExternalId() string                   { return a.ExternalId }
func (a *Actor) GetPermissions() []authschema.Permission { return a.Permissions }
func (a *Actor) GetLabels() map[string]string            { return a.Labels }
func (a *Actor) GetAnnotations() map[string]string       { return a.Annotations }
func (a *Actor) GetCreatedAt() time.Time                 { return a.CreatedAt }
func (a *Actor) GetUpdatedAt() time.Time                 { return a.UpdatedAt }
func (a *Actor) GetResource() *actorschema.Actor {
	return actorResourceFromDatabase(a.Actor)
}
func (a *Actor) Logger() *slog.Logger { return a.logger }

var _ iface.Actor = (*Actor)(nil)
var _ database.IActorDataExtended = (*Actor)(nil)
var _ aplog.HasLogger = (*Actor)(nil)
