package pagination

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
)

// CursorEncryptor is the interface used by pagination to encrypt and decrypt cursor values.
type CursorEncryptor interface {
	EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error)
	Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error)
}

// syntheticEkvID is a fixed ID used by the default cursor encryptor since it doesn't
// participate in the key versioning system.
const syntheticEkvID = "ekv_cursor_default"

// DefaultCursorEncryptor is a simple AES-GCM encryptor that uses a raw key for cursor
// encryption. It does not participate in the key rotation system.
type DefaultCursorEncryptor struct {
	key []byte
}

// NewDefaultCursorEncryptor creates a DefaultCursorEncryptor with the given 32-byte AES key.
func NewDefaultCursorEncryptor(key []byte) *DefaultCursorEncryptor {
	return &DefaultCursorEncryptor{key: key}
}

// NewRandomCursorEncryptor creates a DefaultCursorEncryptor with a random 32-byte AES key.
func NewRandomCursorEncryptor() *DefaultCursorEncryptor {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("failed to generate random cursor key: %v", err))
	}
	return &DefaultCursorEncryptor{key: key}
}

func (e *DefaultCursorEncryptor) EncryptGlobal(_ context.Context, data []byte) (encfield.EncryptedField, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encfield.EncryptedField{}, err
	}

	ciphertext := aesGCM.Seal(nil, nonce, data, nil)
	combined := append(nonce, ciphertext...)

	return encfield.EncryptedField{
		ID:   apid.ID(syntheticEkvID),
		Data: base64.StdEncoding.EncodeToString(combined),
	}, nil
}

func (e *DefaultCursorEncryptor) Decrypt(_ context.Context, ef encfield.EncryptedField) ([]byte, error) {
	combined, err := base64.StdEncoding.DecodeString(ef.Data)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(combined) < nonceSize {
		return nil, fmt.Errorf("invalid encrypted data length")
	}

	nonce, ciphertext := combined[:nonceSize], combined[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}
