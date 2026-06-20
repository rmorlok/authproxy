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
	// EncryptGlobal encrypts the given data using the global encryption key.
	EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error)

	// EncryptStringGlobal encrypts the given data using the global encryption key.
	EncryptStringGlobal(ctx context.Context, data string) (encfield.EncryptedField, error)

	// EncryptForNamespace encrypts the given data using the namespace encryption key.
	EncryptForNamespace(ctx context.Context, namespacePath string, data []byte) (encfield.EncryptedField, error)

	// EncryptStringForNamespace encrypts the given data using the namespace encryption key.
	EncryptStringForNamespace(ctx context.Context, namespacePath string, data string) (encfield.EncryptedField, error)

	// EncryptKeyForNamespace is a special variant for key entities only. It follows special
	// rules about which encryption key to use to avoid cycles in the graph.
	EncryptKeyForNamespace(ctx context.Context, namespacePath string, keyData []byte) (encfield.EncryptedField, error)

	// EncryptForEntity encrypts the given data using the key for namespace of the entity
	EncryptForEntity(ctx context.Context, entity NamespacedEntity, data []byte) (encfield.EncryptedField, error)

	// EncryptStringForEntity encrypts the given data using the key for namespace of the entity
	EncryptStringForEntity(ctx context.Context, enity NamespacedEntity, data string) (encfield.EncryptedField, error)

	// Decrypt decrypts the given encrypted field. It uses the metadata in the encrypted field to identify the
	// appropriate DEK to use for decryption.
	Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error)

	// DecryptString decrypts the given encrypted field to a string. It uses the metadata in the encrypted
	// to identify the appropriate DEK to use for decryption.
	DecryptString(ctx context.Context, ef encfield.EncryptedField) (string, error)

	// ReEncryptField decrypts the given encrypted field and re-encrypts it with the specified
	// target DEK. If the field is already encrypted with the target DEK, it is
	// returned unchanged.
	ReEncryptField(ctx context.Context, ef encfield.EncryptedField, targetDEKId apid.ID) (encfield.EncryptedField, error)

	// SyncKeysFromDbToMemory forces a refresh of the in-memory key caches from the database.
	SyncKeysFromDbToMemory(ctx context.Context) error

	// Start launches the background key sync goroutine.
	Start()

	// Shutdown stops the background key sync goroutine and waits for it to exit.
	Shutdown()
}
