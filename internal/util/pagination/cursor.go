package pagination

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/encfield"
)

// MakeCursor constructs a cursor string from the JSON encoding of the passed value. The cursor string is encrypted
// and base64 encoded so that it cannot be manipulated in the client
func MakeCursor(ctx context.Context, enc CursorEncryptor, c interface{}) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal cursor")
	}

	ef, err := enc.EncryptGlobal(ctx, data)
	if err != nil {
		return "", errors.Wrap(err, "failed to encrypt cursor")
	}

	return ef.ToInlineString(), nil
}

// ParseCursor parses a cursor from the passed value. The passed valued should be generated from MakeCursor
func ParseCursor[C any](ctx context.Context, enc CursorEncryptor, c string) (*C, error) {
	ef, err := encfield.ParseInlineString(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse cursor inline string")
	}

	data, err := enc.Decrypt(ctx, ef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt cursor")
	}

	result := new(C)
	if err := json.Unmarshal(data, result); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal cursor")
	}

	return result, nil
}
