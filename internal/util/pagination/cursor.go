package pagination

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema/config"
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

// MakeCursorMultiKey constructs a cursor string encrypted with the primary key and prepends
// a version prefix for multi-key support.
func MakeCursorMultiKey(ctx context.Context, keys []*config.KeyData, c interface{}) (string, error) {
	if len(keys) == 0 {
		return "", errors.New("no keys provided for cursor encryption")
	}

	keyData, err := keys[0].GetData(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get primary key data to sign cursor")
	}

	return util.SecureEncryptedJsonValueVersioned(0, keyData, c)
}

// ParseCursor parses a cursor from the passed value. The passed valued should be generated from makeCursor
func ParseCursor[C any](ctx context.Context, secretKey config.KeyDataType, c string) (*C, error) {
	keyData, err := secretKey.GetData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret key data to sign cursor")
	}
	return util.SecureDecryptedJsonValue[C](keyData, c)
}

// ParseCursorMultiKey parses a cursor that may be in versioned or legacy format,
// trying all provided keys for legacy cursors.
func ParseCursorMultiKey[C any](ctx context.Context, keys []*config.KeyData, c string) (*C, error) {
	if len(keys) == 0 {
		return nil, errors.New("no keys provided for cursor decryption")
	}

	keyDatas := make([][]byte, 0, len(keys))
	for _, key := range keys {
		data, err := key.GetData(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get key data for cursor decryption")
		}
		keyDatas = append(keyDatas, data)
	}

	return util.SecureDecryptedJsonValueMultiKey[C](keyDatas, c)
}
