package sync

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

// adminActorData implements database.IActorDataExtended for admin user sync operations.
type adminActorData struct {
	id           uuid.UUID
	namespace    string
	externalId   string
	email        string
	permissions  []aschema.Permission
	admin        bool
	superAdmin   bool
	labels       database.Labels
	encryptedKey *string
}

func (a *adminActorData) GetId() uuid.UUID {
	return a.id
}

func (a *adminActorData) GetExternalId() string {
	return a.externalId
}

func (a *adminActorData) GetPermissions() []aschema.Permission {
	return a.permissions
}

func (a *adminActorData) IsAdmin() bool {
	return a.admin
}

func (a *adminActorData) IsSuperAdmin() bool {
	return a.superAdmin
}

func (a *adminActorData) GetEmail() string {
	return a.email
}

func (a *adminActorData) GetNamespace() string {
	return a.namespace
}

func (a *adminActorData) GetLabels() map[string]string {
	return a.labels
}

func (a *adminActorData) GetEncryptedKey() *string {
	return a.encryptedKey
}

// Compile-time check that adminActorData implements IActorDataExtended
var _ database.IActorDataExtended = (*adminActorData)(nil)
