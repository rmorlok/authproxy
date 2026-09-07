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
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// Key is the core abstraction around encryption keys.
type Key struct {
	database.Key

	s      *service
	logger *slog.Logger
}

func wrapKey(ek database.Key, s *service) *Key {
	return &Key{
		Key: ek,
		s:   s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(ek.Namespace).
			Build(),
	}
}

// keyResourceFromDatabase converts the flat persistence model at the core
// boundary. Encrypted provider configuration is deliberately not decrypted or
// copied here; callers that are authorized to return it must load and redact it
// explicitly.
func keyResourceFromDatabase(ek database.Key) *keyschema.Key {
	createdAt := ek.CreatedAt
	updatedAt := ek.UpdatedAt
	return &keyschema.Key{
		TypeMeta: meta.NewTypeMeta(keyschema.KeyKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          ek.Id.String(),
			Name:        ek.Name,
			Namespace:   ek.Namespace,
			Labels:      maps.Clone(map[string]string(ek.Labels)),
			Annotations: maps.Clone(map[string]string(ek.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: keyschema.KeySpec{
			Usage:        keyschema.KeyUsage(ek.Usage),
			MaterialType: keyschema.KeyMaterialType(ek.MaterialType),
			DesiredState: keyschema.KeyState(ek.State),
		},
		Status: &keyschema.KeyStatus{
			State:             keyschema.KeyState(ek.State),
			KeyDataConfigured: ek.EncryptedKeyData != nil && !ek.EncryptedKeyData.IsZero(),
		},
	}
}

// databaseKeyFromResource converts desired resource data into the flat
// database model used to create a key. Provider configuration is encrypted by
// the service and assigned separately.
func databaseKeyFromResource(resource *keyschema.Key, id apid.ID) (*database.Key, error) {
	if resource == nil {
		return nil, fmt.Errorf("key is required")
	}
	return &database.Key{
		Id:           id,
		Namespace:    resource.Metadata.Namespace,
		Name:         resource.Metadata.Name,
		Usage:        database.KeyUsage(resource.Spec.Usage),
		MaterialType: database.KeyMaterialType(resource.Spec.MaterialType),
		State:        database.KeyState(resource.Spec.DesiredState),
		Labels:       database.Labels(maps.Clone(resource.Metadata.Labels)),
		Annotations:  database.Annotations(maps.Clone(resource.Metadata.Annotations)),
	}, nil
}

func (ek *Key) GetId() apid.ID {
	return ek.Id
}

func (ek *Key) GetNamespace() string {
	return ek.Namespace
}

func (ek *Key) GetName() scommon.ResourceName {
	return ek.Name
}

func (ek *Key) GetState() keyschema.KeyState {
	return keyschema.KeyState(ek.State)
}

func (ek *Key) GetCreatedAt() time.Time {
	return ek.CreatedAt
}

func (ek *Key) GetUpdatedAt() time.Time {
	return ek.UpdatedAt
}

func (ek *Key) GetLabels() map[string]string {
	return ek.Labels
}

func (ek *Key) GetAnnotations() map[string]string {
	return ek.Annotations
}

func (ek *Key) GetResource() *keyschema.Key {
	return keyResourceFromDatabase(ek.Key)
}

func (ek *Key) Logger() *slog.Logger {
	return ek.logger
}

var _ iface.Key = (*Key)(nil)
var _ aplog.HasLogger = (*Key)(nil)
