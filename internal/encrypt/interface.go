package encrypt

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
)

type NamespacedEntity interface {
	GetNamespace() string
}

//go:generate mockgen -source=./interface.go -destination=./mock/encrypt.go -package=mock
type E interface {
	EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error)
	EncryptStringGlobal(ctx context.Context, data string) (encfield.EncryptedField, error)
	EncryptForKey(ctx context.Context, keyId apid.ID, data []byte) (encfield.EncryptedField, error)
	EncryptStringForKey(ctx context.Context, keyId apid.ID, data string) (encfield.EncryptedField, error)
	EncryptForNamespace(ctx context.Context, namespacePath string, data []byte) (encfield.EncryptedField, error)
	EncryptStringForNamespace(ctx context.Context, namespacePath string, data string) (encfield.EncryptedField, error)
	EncryptForEntity(ctx context.Context, entity NamespacedEntity, data []byte) (encfield.EncryptedField, error)
	EncryptStringForEntity(ctx context.Context, enity NamespacedEntity, data string) (encfield.EncryptedField, error)
	Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error)
	DecryptString(ctx context.Context, ef encfield.EncryptedField) (string, error)

	// ReEncryptField decrypts the given encrypted field and re-encrypts it with the specified
	// target key version. If the field is already encrypted with the target version, it is
	// returned unchanged.
	ReEncryptField(ctx context.Context, ef encfield.EncryptedField, targetEkvId apid.ID) (encfield.EncryptedField, error)

	// SyncKeysFromDbToMemory forces a refresh of the in-memory key caches from the database.
	SyncKeysFromDbToMemory(ctx context.Context) error

	// Start launches the background key sync goroutine.
	Start()

	// Shutdown stops the background key sync goroutine and waits for it to exit.
	Shutdown()
}
