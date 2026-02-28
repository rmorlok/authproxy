package encrypt

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
)

type ConnectorVersion interface {
	GetId() apid.ID
	GetNamespace() string
	GetVersion() uint64
}

type Connection interface {
	GetId() apid.ID
	GetNamespace() string
	GetConnectorId() apid.ID
	GetConnectorVersion() uint64
}

//go:generate mockgen -source=./interface.go -destination=./mock/encrypt.go -package=mock
type E interface {
	EncryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	EncryptStringGlobal(ctx context.Context, data string) (string, error)
	EncryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error)
	EncryptStringForConnection(ctx context.Context, connection Connection, data string) (string, error)
	EncryptForConnector(ctx context.Context, connection ConnectorVersion, data []byte) ([]byte, error)
	EncryptStringForConnector(ctx context.Context, connection ConnectorVersion, data string) (string, error)
	DecryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	DecryptStringGlobal(ctx context.Context, base64 string) (string, error)
	DecryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error)
	DecryptStringForConnection(ctx context.Context, connection Connection, base64 string) (string, error)
	DecryptForConnector(ctx context.Context, connection ConnectorVersion, data []byte) ([]byte, error)
	DecryptStringForConnector(ctx context.Context, connection ConnectorVersion, base64 string) (string, error)

	// IsEncryptedWithPrimaryKey checks whether a string-encrypted value was encrypted with the
	// primary (index 0) key. Values with "v1:0:" prefix are considered primary-key encrypted.
	// Legacy values (no prefix) return false.
	IsEncryptedWithPrimaryKey(base64Str string) bool
}
