package config

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
)

const ProviderTypeMockKMS ProviderType = "mock_kms"

var (
	keyDataMockKMSRegistryMu sync.RWMutex
	keyDataMockKMSRegistry   = make(map[string]*keyDataMockKMSEntry)
)

type keyDataMockKMSEntry struct {
	versions []keyDataMockKMSVersion
}

type keyDataMockKMSVersion struct {
	ProviderID       string
	ProviderVersion  string
	KeyEncryptionKey []byte
	IsCurrent        bool
}

// KeyDataMockKMS behaves like a KMS-backed KeyData provider for tests. It never
// exposes the KEK as KeyVersionInfo.Data; instead, it generates local DEKs,
// wraps them with the current mock KEK, and persists the wrapped DEK in
// KeyVersionProtectedData.
type KeyDataMockKMS struct {
	MockKMSID string `json:"mock_kms_id" yaml:"mock_kms_id"`
}

func ResetKeyDataMockKMSRegistry() {
	keyDataMockKMSRegistryMu.Lock()
	defer keyDataMockKMSRegistryMu.Unlock()
	keyDataMockKMSRegistry = make(map[string]*keyDataMockKMSEntry)
}

func NewKeyDataMockKMS(mockID string) *KeyData {
	keyDataMockKMSRegistryMu.Lock()
	defer keyDataMockKMSRegistryMu.Unlock()

	if _, exists := keyDataMockKMSRegistry[mockID]; !exists {
		keyDataMockKMSRegistry[mockID] = &keyDataMockKMSEntry{}
	}

	return &KeyData{InnerVal: &KeyDataMockKMS{MockKMSID: mockID}}
}

func KeyDataMockKMSAddVersion(mockID, providerID, providerVersion string, keyEncryptionKey []byte) {
	keyDataMockKMSRegistryMu.Lock()
	defer keyDataMockKMSRegistryMu.Unlock()

	entry, ok := keyDataMockKMSRegistry[mockID]
	if !ok {
		entry = &keyDataMockKMSEntry{}
		keyDataMockKMSRegistry[mockID] = entry
	}

	for i := range entry.versions {
		entry.versions[i].IsCurrent = false
	}

	entry.versions = append(entry.versions, keyDataMockKMSVersion{
		ProviderID:       providerID,
		ProviderVersion:  providerVersion,
		KeyEncryptionKey: keyEncryptionKey,
		IsCurrent:        true,
	})
}

func (m *KeyDataMockKMS) getEntry() (*keyDataMockKMSEntry, error) {
	keyDataMockKMSRegistryMu.RLock()
	defer keyDataMockKMSRegistryMu.RUnlock()

	entry, ok := keyDataMockKMSRegistry[m.MockKMSID]
	if !ok {
		return nil, fmt.Errorf("mock kms key data %q not found in registry", m.MockKMSID)
	}

	copied := &keyDataMockKMSEntry{versions: make([]keyDataMockKMSVersion, len(entry.versions))}
	copy(copied.versions, entry.versions)
	return copied, nil
}

func (m *KeyDataMockKMS) currentVersion() (keyDataMockKMSVersion, error) {
	entry, err := m.getEntry()
	if err != nil {
		return keyDataMockKMSVersion{}, err
	}

	for _, v := range entry.versions {
		if v.IsCurrent {
			return v, nil
		}
	}

	return keyDataMockKMSVersion{}, fmt.Errorf("no current version for mock kms key data %q", m.MockKMSID)
}

func (m *KeyDataMockKMS) version(providerID, providerVersion string) (keyDataMockKMSVersion, error) {
	entry, err := m.getEntry()
	if err != nil {
		return keyDataMockKMSVersion{}, err
	}

	for _, v := range entry.versions {
		if v.ProviderID == providerID && v.ProviderVersion == providerVersion {
			return v, nil
		}
	}

	return keyDataMockKMSVersion{}, fmt.Errorf("mock kms version %q/%q not found for %q", providerID, providerVersion, m.MockKMSID)
}

func (m *KeyDataMockKMS) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	versions, err := m.ListVersionsWithExisting(ctx, nil)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	for _, v := range versions {
		if v.IsCurrent {
			return v, nil
		}
	}

	return KeyVersionInfo{}, fmt.Errorf("no current version for mock kms key data %q", m.MockKMSID)
}

func (m *KeyDataMockKMS) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	versions, err := m.ListVersionsWithExisting(ctx, nil)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	for _, v := range versions {
		if v.ProviderVersion == version {
			return v, nil
		}
	}

	return KeyVersionInfo{}, fmt.Errorf("version %q not found for mock kms key data %q", version, m.MockKMSID)
}

func (m *KeyDataMockKMS) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	return m.ListVersionsWithExisting(ctx, nil)
}

func (m *KeyDataMockKMS) ListVersionsWithExisting(_ context.Context, existing []ExistingKeyVersionInfo) ([]KeyVersionInfo, error) {
	current, err := m.currentVersion()
	if err != nil {
		return nil, err
	}

	var result []KeyVersionInfo
	hasCurrent := false
	for _, ex := range existing {
		if ex.Provider != ProviderTypeMockKMS {
			continue
		}
		if ex.ProtectedData == nil || ex.ProtectedData.IsZero() {
			continue
		}

		kekVersion := ex.ProtectedData.Metadata["kek_version"]
		if kekVersion == "" {
			kekVersion = ex.ProviderVersion
		}

		kek, err := m.version(ex.ProviderID, kekVersion)
		if err != nil {
			return nil, err
		}

		dek, err := mockKMSUnwrap(kek.KeyEncryptionKey, *ex.ProtectedData)
		if err != nil {
			return nil, err
		}

		isCurrent := ex.ProviderID == current.ProviderID && ex.ProviderVersion == current.ProviderVersion
		if isCurrent {
			hasCurrent = true
		}
		result = append(result, KeyVersionInfo{
			Provider:        ProviderTypeMockKMS,
			ProviderID:      ex.ProviderID,
			ProviderVersion: ex.ProviderVersion,
			Data:            dek,
			ProtectedData:   ex.ProtectedData,
			IsCurrent:       isCurrent,
		})
	}

	if !hasCurrent {
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			return nil, fmt.Errorf("failed to generate mock kms data key: %w", err)
		}

		protected, err := mockKMSWrap(current.KeyEncryptionKey, current.ProviderVersion, dek)
		if err != nil {
			return nil, err
		}

		result = append(result, KeyVersionInfo{
			Provider:        ProviderTypeMockKMS,
			ProviderID:      current.ProviderID,
			ProviderVersion: current.ProviderVersion,
			Data:            dek,
			ProtectedData:   &protected,
			IsCurrent:       true,
		})
	}

	return result, nil
}

func (m *KeyDataMockKMS) GetProviderType() ProviderType {
	return ProviderTypeMockKMS
}

func mockKMSWrap(kek []byte, kekVersion string, dek []byte) (KeyVersionProtectedData, error) {
	encrypted, err := mockKMSCrypt(kek, dek)
	if err != nil {
		return KeyVersionProtectedData{}, err
	}

	return KeyVersionProtectedData{
		Type:        string(ProviderTypeMockKMS),
		WrappedData: base64.StdEncoding.EncodeToString(encrypted),
		Metadata: map[string]string{
			"kek_version": kekVersion,
		},
	}, nil
}

func mockKMSUnwrap(kek []byte, protected KeyVersionProtectedData) ([]byte, error) {
	if protected.Type != string(ProviderTypeMockKMS) {
		return nil, fmt.Errorf("unsupported mock kms protected data type %q", protected.Type)
	}

	decoded, err := base64.StdEncoding.DecodeString(protected.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode mock kms wrapped data: %w", err)
	}

	return mockKMSDecrypt(kek, decoded)
}

func mockKMSCrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create mock kms cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create mock kms gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate mock kms nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func mockKMSDecrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create mock kms cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create mock kms gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("mock kms wrapped data is too short")
	}

	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

var _ KeyDataType = (*KeyDataMockKMS)(nil)
var _ KeyDataTypeWithExistingVersions = (*KeyDataMockKMS)(nil)
