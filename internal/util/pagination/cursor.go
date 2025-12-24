package pagination

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/util"
)

// MakeCursor constructs a cursor string from the JSON encoding of the passed value. The cursor string is encrypted
// and base64 encoded so that it cannot be manipulated in the client
func MakeCursor(ctx context.Context, secretKey config.KeyDataType, c interface{}) (string, error) {
	keyData, err := secretKey.GetData(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get secret key data to sign cursor")
	}
	return util.SecureEncryptedJsonValue(keyData, c)
}

// ParseCursor parses a cursor from the passed value. The passed valued should be generated from makeCursor
func ParseCursor[C any](ctx context.Context, secretKey config.KeyDataType, c string) (*C, error) {
	keyData, err := secretKey.GetData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret key data to sign cursor")
	}
	return util.SecureDecryptedJsonValue[C](keyData, c)
}
