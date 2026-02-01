package sync

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

// configuredActorData implements database.IActorDataExtended for configured actor sync operations.
type configuredActorData struct {
	id           uuid.UUID
	namespace    string
	externalId   string
	permissions  []aschema.Permission
	labels       database.Labels
	encryptedKey *string
}

func (a *configuredActorData) GetId() uuid.UUID {
	return a.id
}

func (a *configuredActorData) GetExternalId() string {
	return a.externalId
}

func (a *configuredActorData) GetPermissions() []aschema.Permission {
	return a.permissions
}

func (a *configuredActorData) GetNamespace() string {
	return a.namespace
}

func (a *configuredActorData) GetLabels() map[string]string {
	return a.labels
}

func (a *configuredActorData) GetEncryptedKey() *string {
	return a.encryptedKey
}

// Compile-time check that configuredActorData implements IActorDataExtended
var _ database.IActorDataExtended = (*configuredActorData)(nil)
