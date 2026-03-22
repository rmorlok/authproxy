package pagination

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/encfield"
)

// MakeCursor constructs a cursor string from the JSON encoding of the passed value. The cursor string is encrypted
// and base64 encoded so that it cannot be manipulated in the client
func MakeCursor(ctx context.Context, enc CursorEncryptor, c interface{}) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor: %w", err)
	}

	ef, err := enc.EncryptGlobal(ctx, data)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt cursor: %w", err)
	}

	return ef.ToInlineString(), nil
}

// ParseCursor parses a cursor from the passed value. The passed valued should be generated from MakeCursor
func ParseCursor[C any](ctx context.Context, enc CursorEncryptor, c string) (*C, error) {
	ef, err := encfield.ParseInlineString(c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cursor inline string: %w", err)
	}

	data, err := enc.Decrypt(ctx, ef)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt cursor: %w", err)
	}

	result := new(C)
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	return result, nil
}
