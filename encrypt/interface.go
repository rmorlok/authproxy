package encrypt

import (
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
)

/*
 * Regenerate mocks for this interface using:
 * mockgen -source=encrypt/interface.go -destination=encrypt/mock/encrypt.go -package=mock
 */

type E interface {
	EncryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	EncryptStringGlobal(ctx context.Context, data string) (string, error)
	EncryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error)
	EncryptStringForConnection(ctx context.Context, connection database.Connection, data string) (string, error)
	DecryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	DecryptStringGlobal(ctx context.Context, base64 string) (string, error)
	DecryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error)
	DecryptStringForConnection(ctx context.Context, connection database.Connection, base64 string) (string, error)
}
