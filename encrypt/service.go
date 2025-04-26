package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"io"
	"sync"
)

type service struct {
	cfg config.C
	db  database.DB

	globalAESKey     []byte
	globalAESKeyErr  error
	globalAESKeyOnce sync.Once
}

func NewTestEncryptService(
	cfg config.C,
	db database.DB,
) (config.C, E) {
	if cfg == nil {
		cfg = config.FromRoot(&config.Root{})
	}

	if cfg.GetRoot().SystemAuth.GlobalAESKey == nil {
		cfg.GetRoot().SystemAuth.GlobalAESKey = &config.KeyDataRandomBytes{}
	}

	return cfg, &service{
		cfg: cfg,
		db:  db,
	}
}

func NewEncryptService(
	cfg config.C,
	db database.DB,
) E {
	return &service{
		cfg: cfg,
		db:  db,
	}
}

func (s *service) getGlobalAESKey(ctx context.Context) ([]byte, error) {
	s.globalAESKeyOnce.Do(func() {
		if s == nil ||
			s.cfg == nil ||
			s.cfg.GetRoot() == nil ||
			s.cfg.GetRoot().SystemAuth.GlobalAESKey == nil ||
			!s.cfg.GetRoot().SystemAuth.GlobalAESKey.HasData(ctx) {
			s.globalAESKey = nil
			s.globalAESKeyErr = errors.New("no global AES key")
			return
		}

		s.globalAESKey, s.globalAESKeyErr = s.cfg.GetRoot().SystemAuth.GlobalAESKey.GetData(ctx)
	})

	return s.globalAESKey, s.globalAESKeyErr
}

func (s *service) EncryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	globalAESKey, err := s.getGlobalAESKey(ctx)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(globalAESKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCM")
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "failed to generate nonce")
	}

	encryptedData := gcm.Seal(nonce, nonce, data, nil)
	return encryptedData, nil
}

func (s *service) EncryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *service) DecryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	globalAESKey, err := s.getGlobalAESKey(ctx)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(globalAESKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCM")
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("data length is too short to contain nonce")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decryptedData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decryption failed")
	}

	return decryptedData, nil
}

func (s *service) DecryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

func (s *service) EncryptStringGlobal(ctx context.Context, data string) (string, error) {
	encryptedData, err := s.EncryptGlobal(ctx, []byte(data))
	if err != nil {
		return "", err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encodedData, nil
}

func (s *service) EncryptStringForConnection(ctx context.Context, connection database.Connection, data string) (string, error) {
	encryptedData, err := s.EncryptForConnection(ctx, connection, []byte(data))
	if err != nil {
		return "", err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encodedData, nil
}

func (s *service) DecryptStringGlobal(ctx context.Context, base64Data string) (string, error) {
	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := s.DecryptGlobal(ctx, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *service) DecryptStringForConnection(ctx context.Context, connection database.Connection, base64Data string) (string, error) {
	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := s.DecryptForConnection(ctx, connection, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}
