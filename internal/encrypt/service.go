package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// versionedPrefix is the prefix for versioned encrypted strings.
// Format: "v1:<keyIndex>:<base64>"
const versionedPrefix = "v1:"

type service struct {
	cfg config.C
	db  database.DB

	globalAESKeys    [][]byte
	globalAESKeysErr error
	globalAESKeysOnce sync.Once
}

func NewTestEncryptService(
	cfg config.C,
	db database.DB,
) (config.C, E) {
	if cfg == nil {
		cfg = config.FromRoot(&sconfig.Root{})
	}

	if cfg.GetRoot().SystemAuth.GlobalAESKey == nil && len(cfg.GetRoot().SystemAuth.GlobalAESKeys) == 0 {
		cfg.GetRoot().SystemAuth.GlobalAESKey = &sconfig.KeyData{InnerVal: &sconfig.KeyDataRandomBytes{}}
	}

	return cfg, NewEncryptService(cfg, db)
}

func NewEncryptService(
	cfg config.C,
	db database.DB,
) E {
	if cfg != nil && cfg.GetRoot().DevSettings.IsFakeEncryptionEnabled() {
		doBase64Encode := !cfg.GetRoot().DevSettings.IsFakeEncryptionSkipBase64Enabled()
		return NewFakeEncryptService(doBase64Encode)
	}

	return &service{
		cfg: cfg,
		db:  db,
	}
}

// getGlobalAESKeys returns all configured AES keys. The primary key is at index 0.
func (s *service) getGlobalAESKeys(ctx context.Context) ([][]byte, error) {
	s.globalAESKeysOnce.Do(func() {
		if s == nil || s.cfg == nil || s.cfg.GetRoot() == nil {
			s.globalAESKeysErr = errors.New("no global AES key configured")
			return
		}

		keyDatas := s.cfg.GetRoot().SystemAuth.GetGlobalAESKeys()
		if len(keyDatas) == 0 {
			s.globalAESKeysErr = errors.New("no global AES key configured")
			return
		}

		keys := make([][]byte, 0, len(keyDatas))
		for i, kd := range keyDatas {
			if kd == nil || !kd.HasData(ctx) {
				s.globalAESKeysErr = fmt.Errorf("global AES key at index %d has no data", i)
				return
			}
			data, err := kd.GetData(ctx)
			if err != nil {
				s.globalAESKeysErr = errors.Wrapf(err, "failed to get global AES key at index %d", i)
				return
			}
			keys = append(keys, data)
		}

		s.globalAESKeys = keys
	})

	return s.globalAESKeys, s.globalAESKeysErr
}

// getPrimaryKey returns the primary (first) AES key.
func (s *service) getPrimaryKey(ctx context.Context) ([]byte, error) {
	keys, err := s.getGlobalAESKeys(ctx)
	if err != nil {
		return nil, err
	}
	return keys[0], nil
}

func encryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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

func decryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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

// decryptWithAnyKey tries to decrypt data with each key in order, returning the first success.
func decryptWithAnyKey(keys [][]byte, data []byte) ([]byte, error) {
	var lastErr error
	for _, key := range keys {
		result, err := decryptWithKey(key, data)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, errors.Wrap(lastErr, "decryption failed with all keys")
}

// EncryptGlobal encrypts raw bytes with the primary key.
// For raw byte methods, no version prefix is added (used for ephemeral data like session cookies).
func (s *service) EncryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	key, err := s.getPrimaryKey(ctx)
	if err != nil {
		return nil, err
	}
	return encryptWithKey(key, data)
}

func (s *service) EncryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *service) EncryptForConnector(ctx context.Context, connection ConnectorVersion, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

// DecryptGlobal decrypts raw bytes by trying all keys.
// For raw byte methods, there is no version prefix (used for ephemeral data).
func (s *service) DecryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	keys, err := s.getGlobalAESKeys(ctx)
	if err != nil {
		return nil, err
	}
	return decryptWithAnyKey(keys, data)
}

func (s *service) DecryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

func (s *service) DecryptForConnector(ctx context.Context, cv ConnectorVersion, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

// EncryptStringGlobal encrypts a string with the primary key and returns a versioned string.
// Output format: "v1:0:<base64>"
func (s *service) EncryptStringGlobal(ctx context.Context, data string) (string, error) {
	key, err := s.getPrimaryKey(ctx)
	if err != nil {
		return "", err
	}

	encryptedData, err := encryptWithKey(key, []byte(data))
	if err != nil {
		return "", err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return fmt.Sprintf("%s0:%s", versionedPrefix, encodedData), nil
}

func (s *service) EncryptStringForConnection(ctx context.Context, connection Connection, data string) (string, error) {
	return s.EncryptStringGlobal(ctx, data)
}

func (s *service) EncryptStringForConnector(ctx context.Context, cv ConnectorVersion, data string) (string, error) {
	return s.EncryptStringGlobal(ctx, data)
}

// DecryptStringGlobal decrypts a string that may be in versioned or legacy format.
// Versioned format: "v1:<keyIndex>:<base64>"
// Legacy format: "<base64>" (no prefix, tries all keys)
func (s *service) DecryptStringGlobal(ctx context.Context, base64Data string) (string, error) {
	keys, err := s.getGlobalAESKeys(ctx)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(base64Data, versionedPrefix) {
		// Versioned format: "v1:<keyIndex>:<base64>"
		rest := base64Data[len(versionedPrefix):]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			return "", errors.New("invalid versioned encrypted string: missing key index separator")
		}

		keyIndexStr := rest[:colonIdx]
		encodedData := rest[colonIdx+1:]

		keyIndex, err := strconv.Atoi(keyIndexStr)
		if err != nil {
			return "", errors.Wrap(err, "invalid key index in versioned encrypted string")
		}

		if keyIndex < 0 || keyIndex >= len(keys) {
			return "", fmt.Errorf("key index %d out of range (have %d keys)", keyIndex, len(keys))
		}

		decodedData, err := base64.StdEncoding.DecodeString(encodedData)
		if err != nil {
			return "", errors.Wrap(err, "failed to decode base64 string")
		}

		decryptedData, err := decryptWithKey(keys[keyIndex], decodedData)
		if err != nil {
			return "", err
		}

		return string(decryptedData), nil
	}

	// Legacy format: no prefix, try all keys
	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := decryptWithAnyKey(keys, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *service) DecryptStringForConnection(ctx context.Context, connection Connection, base64Data string) (string, error) {
	return s.DecryptStringGlobal(ctx, base64Data)
}

func (s *service) DecryptStringForConnector(ctx context.Context, cv ConnectorVersion, base64Data string) (string, error) {
	return s.DecryptStringGlobal(ctx, base64Data)
}

// IsEncryptedWithPrimaryKey checks whether a string value was encrypted with the primary key.
// Only versioned strings with key index 0 return true.
func (s *service) IsEncryptedWithPrimaryKey(base64Str string) bool {
	return strings.HasPrefix(base64Str, "v1:0:")
}
