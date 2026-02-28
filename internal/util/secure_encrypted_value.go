package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func MustGenerateSecureRandomKey(size int) []byte {
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}

// SecureEncryptedJsonValue serializes an arbitrary structure to json, encrypts the data using a symmetric key,
// then returns a base64 encode value. This can be used to send values to the client in a way that cannot be
// manipulated, but allows for easy structured data.
//
// The key argument should be the AES key, either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256.
func SecureEncryptedJsonValue(key []byte, val interface{}) (string, error) {
	jsonData, err := json.Marshal(val)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// AES-GCM needs a unique nonce for every encryption
	nonce := make([]byte, 12) // 12 bytes is the recommended nonce size for GCM
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Encrypt the plaintext
	ciphertext := aesGCM.Seal(nil, nonce, jsonData, nil)

	// Combine nonce and ciphertext
	combined := append(nonce, ciphertext...)

	// Encode the combined data as Base64
	encoded := base64.StdEncoding.EncodeToString(combined)

	return encoded, nil
}

// SecureEncryptedJsonValueVersioned encrypts a JSON value with the specified key and prepends
// a version prefix "v1:<keyIndex>:" to the output.
func SecureEncryptedJsonValueVersioned(keyIndex int, key []byte, val interface{}) (string, error) {
	encoded, err := SecureEncryptedJsonValue(key, val)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v1:%d:%s", keyIndex, encoded), nil
}

// SecureDecryptedJsonValueMultiKey decrypts a JSON value that may be in versioned or legacy format.
// Versioned format: "v1:<keyIndex>:<base64>"
// Legacy format: "<base64>" (tries all keys)
func SecureDecryptedJsonValueMultiKey[T any](keys [][]byte, encoded string) (*T, error) {
	if strings.HasPrefix(encoded, "v1:") {
		rest := encoded[3:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("invalid versioned format: missing key index separator")
		}

		keyIndexStr := rest[:colonIdx]
		base64Data := rest[colonIdx+1:]

		keyIndex, err := strconv.Atoi(keyIndexStr)
		if err != nil {
			return nil, fmt.Errorf("invalid key index: %w", err)
		}

		if keyIndex < 0 || keyIndex >= len(keys) {
			return nil, fmt.Errorf("key index %d out of range (have %d keys)", keyIndex, len(keys))
		}

		return SecureDecryptedJsonValue[T](keys[keyIndex], base64Data)
	}

	// Legacy format: try all keys
	var lastErr error
	for _, key := range keys {
		result, err := SecureDecryptedJsonValue[T](key, encoded)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("decryption failed with all keys: %w", lastErr)
}

func SecureDecryptedJsonValue[T any](key []byte, encoded string) (*T, error) {
	// Decode the Base64 string
	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	// Split the nonce and ciphertext
	nonceSize := 12 // AES-GCM standard nonce size
	if len(combined) < nonceSize {
		return nil, fmt.Errorf("invalid data length")
	}
	nonce, ciphertext := combined[:nonceSize], combined[nonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt the ciphertext
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	t := new(T)
	err = json.Unmarshal(plaintext, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}
