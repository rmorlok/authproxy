package encrypt

import (
	"context"
	"github.com/rmorlok/authproxy/database"
)

//go:generate mockgen -source=./interface.go -destination=./mock/encrypt.go -package=mock
type E interface {
	EncryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	EncryptStringGlobal(ctx context.Context, data string) (string, error)
	EncryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error)
	EncryptStringForConnection(ctx context.Context, connection database.Connection, data string) (string, error)
	EncryptForConnector(ctx context.Context, connection database.ConnectorVersion, data []byte) ([]byte, error)
	EncryptStringForConnector(ctx context.Context, connection database.ConnectorVersion, data string) (string, error)
	DecryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	DecryptStringGlobal(ctx context.Context, base64 string) (string, error)
	DecryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error)
	DecryptStringForConnection(ctx context.Context, connection database.Connection, base64 string) (string, error)
	DecryptForConnector(ctx context.Context, connection database.ConnectorVersion, data []byte) ([]byte, error)
	DecryptStringForConnector(ctx context.Context, connection database.ConnectorVersion, base64 string) (string, error)
}
