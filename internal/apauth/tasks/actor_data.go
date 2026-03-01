package tasks

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

// configuredActorData implements database.IActorDataExtended for configured actor sync operations.
type configuredActorData struct {
	id           apid.ID
	namespace    string
	externalId   string
	permissions  []aschema.Permission
	labels       database.Labels
	encryptedKey *string
}

func (a *configuredActorData) GetId() apid.ID {
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
