package key

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	// DataEncryptionKeySize is the byte size for AuthProxy-generated DEKs.
	DataEncryptionKeySize = 32

	// KeyVersionProtectedDataTypeAuthProxyAESGCM is the protected-data type used
	// when AuthProxy generates DEK bytes and wraps them with provider key bytes.
	KeyVersionProtectedDataTypeAuthProxyAESGCM = "authproxy_aes_gcm"

	protectedDataMetadataWrappingProvider        = "wrapping_provider"
	protectedDataMetadataWrappingProviderID      = "wrapping_provider_id"
	protectedDataMetadataWrappingProviderVersion = "wrapping_provider_version"
)

// CurrentWrappingKey returns the provider key identity that would be used for
// DEK wrapping right now. Providers that do not manage wrapping themselves derive
// this from their current KeyVersionInfo without exposing key bytes.
func (kd *KeyData) CurrentWrappingKey(ctx context.Context) (KeyWrappingKeyInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return KeyWrappingKeyInfo{}, errors.New("key data is nil")
	}

	if wrapper, ok := kd.InnerVal.(KeyDataWrapsDataEncryptionKeys); ok {
		return wrapper.CurrentWrappingKey(ctx)
	}

	current, err := kd.InnerVal.GetCurrentVersion(ctx)
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}
	if len(current.Data) == 0 {
		return KeyWrappingKeyInfo{}, fmt.Errorf("current key data for provider %q does not expose wrapping bytes", current.Provider)
	}

	return KeyWrappingKeyInfo{
		Provider:        current.Provider,
		ProviderID:      current.ProviderID,
		ProviderVersion: current.ProviderVersion,
	}, nil
}

// GenerateDataEncryptionKey returns a new DEK wrapped by this key data provider.
// Native KMS-style generators can override this through the inner provider;
// otherwise AuthProxy generates 32 random bytes and wraps them through the
// provider's current key material.
func (kd *KeyData) GenerateDataEncryptionKey(ctx context.Context) (GeneratedDataEncryptionKey, error) {
	if kd == nil || kd.InnerVal == nil {
		return GeneratedDataEncryptionKey{}, errors.New("key data is nil")
	}

	if generator, ok := kd.InnerVal.(KeyDataGeneratesDataEncryptionKeys); ok {
		return generator.GenerateDataEncryptionKey(ctx)
	}

	dek, err := generateAuthProxyDataEncryptionKey()
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	return kd.WrapDataEncryptionKey(ctx, dek)
}

// WrapDataEncryptionKey wraps the provided plaintext DEK and returns the
// metadata needed to persist it. Providers with native wrapping can override
// this; ordinary secret-backed providers use AES-GCM with their current key
// material.
func (kd *KeyData) WrapDataEncryptionKey(ctx context.Context, dek []byte) (GeneratedDataEncryptionKey, error) {
	if kd == nil || kd.InnerVal == nil {
		return GeneratedDataEncryptionKey{}, errors.New("key data is nil")
	}
	if len(dek) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("data encryption key is empty")
	}

	if wrapper, ok := kd.InnerVal.(KeyDataWrapsDataEncryptionKeys); ok {
		return wrapper.WrapDataEncryptionKey(ctx, dek)
	}

	current, err := kd.InnerVal.GetCurrentVersion(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	if len(current.Data) == 0 {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("current key data for provider %q does not expose wrapping bytes", current.Provider)
	}

	protected, err := wrapDataEncryptionKeyWithAESGCM(current.Data, current, dek)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	return GeneratedDataEncryptionKey{
		Provider:        current.Provider,
		ProviderID:      current.ProviderID,
		ProviderVersion: current.ProviderVersion,
		ProtectedData:   protected,
		Data:            append([]byte(nil), dek...),
	}, nil
}

// UnwrapDataEncryptionKey returns the plaintext DEK bytes for a persisted DEK
// row. The provider version recorded on the DEK chooses the wrapping key
// version, allowing providers that retain historical key material to unwrap
// older DEKs during rewrap.
func (kd *KeyData) UnwrapDataEncryptionKey(ctx context.Context, dek DataEncryptionKeyInfo) ([]byte, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	if wrapper, ok := kd.InnerVal.(KeyDataWrapsDataEncryptionKeys); ok {
		return wrapper.UnwrapDataEncryptionKey(ctx, dek)
	}

	if dek.ProtectedData == nil || dek.ProtectedData.IsZero() {
		return nil, errors.New("data encryption key protected data is empty")
	}
	if dek.ProtectedData.Type != KeyVersionProtectedDataTypeAuthProxyAESGCM {
		return nil, fmt.Errorf("unsupported protected data type %q", dek.ProtectedData.Type)
	}

	version, err := kd.InnerVal.GetVersion(ctx, dek.ProviderVersion)
	if err != nil {
		return nil, err
	}
	if version.Provider != dek.Provider {
		return nil, fmt.Errorf("data encryption key provider %q does not match wrapping provider %q", dek.Provider, version.Provider)
	}
	if dek.ProviderID != "" && version.ProviderID != dek.ProviderID {
		return nil, fmt.Errorf("data encryption key provider id %q does not match wrapping provider id %q", dek.ProviderID, version.ProviderID)
	}
	if len(version.Data) == 0 {
		return nil, fmt.Errorf("key data for provider %q version %q does not expose wrapping bytes", version.Provider, version.ProviderVersion)
	}

	return unwrapDataEncryptionKeyWithAESGCM(version.Data, *dek.ProtectedData)
}

func generateAuthProxyDataEncryptionKey() ([]byte, error) {
	dek := make([]byte, DataEncryptionKeySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("failed to generate data encryption key: %w", err)
	}
	return dek, nil
}

func wrapDataEncryptionKeyWithAESGCM(
	wrappingKey []byte,
	info KeyVersionInfo,
	dek []byte,
) (KeyVersionProtectedData, error) {
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return KeyVersionProtectedData{}, fmt.Errorf("failed to create DEK wrapping cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return KeyVersionProtectedData{}, fmt.Errorf("failed to create DEK wrapping GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return KeyVersionProtectedData{}, fmt.Errorf("failed to generate DEK wrapping nonce: %w", err)
	}

	encrypted := gcm.Seal(nonce, nonce, dek, nil)
	return KeyVersionProtectedData{
		Type:        KeyVersionProtectedDataTypeAuthProxyAESGCM,
		WrappedData: base64.StdEncoding.EncodeToString(encrypted),
		Metadata: map[string]string{
			protectedDataMetadataWrappingProvider:        string(info.Provider),
			protectedDataMetadataWrappingProviderID:      info.ProviderID,
			protectedDataMetadataWrappingProviderVersion: info.ProviderVersion,
		},
	}, nil
}

func unwrapDataEncryptionKeyWithAESGCM(wrappingKey []byte, protected KeyVersionProtectedData) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(protected.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode wrapped data encryption key: %w", err)
	}

	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create DEK unwrapping cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create DEK unwrapping GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(decoded) < nonceSize {
		return nil, errors.New("wrapped data encryption key is too short")
	}

	return gcm.Open(nil, decoded[:nonceSize], decoded[nonceSize:], nil)
}

var _ KeyDataGeneratesDataEncryptionKeys = (*KeyData)(nil)
var _ KeyDataWrapsDataEncryptionKeys = (*KeyData)(nil)
