package database

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/util"
)

// makeCursor constructs a cursor string from the JSON encoding of the passed value. The cursor string is encrypted
// and base64 encoded so that it cannot be manipulated in the client
func makeCursor(ctx context.Context, secretKey config.KeyData, c interface{}) (string, error) {
	keyData, err := secretKey.GetData(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get secret key data to sign cursor")
	}
	return util.SecureEncryptedJsonValue(keyData, c)
}

// parseCursor parses a cursor from the passed value. The passed valued should be generated from makeCursor
func parseCursor[C any](ctx context.Context, secretKey config.KeyData, c string) (*C, error) {
	keyData, err := secretKey.GetData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret key data to sign cursor")
	}
	return util.SecureDecryptedJsonValue[C](keyData, c)
}
