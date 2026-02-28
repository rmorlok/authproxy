package config

import (
	"context"
	"fmt"
	"sync"
)

const ProviderTypeMock ProviderType = "mock"

// keyDataMockRegistry is a global in-memory registry mapping mock IDs to their version lists.
// This allows mock key data instances to be serialized/deserialized via JSON/YAML while still
// permitting the test to mutate the version list between calls.
var (
	keyDataMockRegistryMu sync.RWMutex
	keyDataMockRegistry   = make(map[string]*keyDataMockEntry)
)

type keyDataMockEntry struct {
	versions []KeyVersionInfo
}

// KeyDataMock is a mock KeyDataType backed by an in-memory registry keyed by MockID.
// It supports JSON/YAML round-tripping: only the MockID is serialized, and the actual
// version data lives in the global registry. This lets tests mutate the set of versions
// between sync calls while child encryption keys can deserialize from encrypted data.
type KeyDataMock struct {
	MockID string `json:"mock_id" yaml:"mock_id"`
}

// ResetKeyDataMockRegistry clears all entries from the global mock registry.
// Call this in test cleanup to prevent state leaking between tests.
func ResetKeyDataMockRegistry() {
	keyDataMockRegistryMu.Lock()
	defer keyDataMockRegistryMu.Unlock()
	keyDataMockRegistry = make(map[string]*keyDataMockEntry)
}

// NewKeyDataMock creates a new KeyDataMock with the given ID and registers it in the
// global registry with an empty version list. Returns a KeyData wrapping the mock.
func NewKeyDataMock(mockID string) *KeyData {
	keyDataMockRegistryMu.Lock()
	defer keyDataMockRegistryMu.Unlock()

	if _, exists := keyDataMockRegistry[mockID]; !exists {
		keyDataMockRegistry[mockID] = &keyDataMockEntry{}
	}

	return &KeyData{InnerVal: &KeyDataMock{MockID: mockID}}
}

// KeyDataMockAddVersion adds a new version to the mock identified by mockID, marking it
// as current and unmarking any previous current version.
func KeyDataMockAddVersion(mockID, providerID, providerVersion string, data []byte) {
	keyDataMockRegistryMu.Lock()
	defer keyDataMockRegistryMu.Unlock()

	entry, ok := keyDataMockRegistry[mockID]
	if !ok {
		entry = &keyDataMockEntry{}
		keyDataMockRegistry[mockID] = entry
	}

	for i := range entry.versions {
		entry.versions[i].IsCurrent = false
	}

	entry.versions = append(entry.versions, KeyVersionInfo{
		Provider:        ProviderTypeMock,
		ProviderID:      providerID,
		ProviderVersion: providerVersion,
		Data:            data,
		IsCurrent:       true,
	})
}

// KeyDataMockSetVersions replaces all versions for the mock identified by mockID.
func KeyDataMockSetVersions(mockID string, versions []KeyVersionInfo) {
	keyDataMockRegistryMu.Lock()
	defer keyDataMockRegistryMu.Unlock()

	entry, ok := keyDataMockRegistry[mockID]
	if !ok {
		entry = &keyDataMockEntry{}
		keyDataMockRegistry[mockID] = entry
	}

	entry.versions = versions
}

// KeyDataMockRemoveVersion removes the version with the given providerVersion from the mock.
func KeyDataMockRemoveVersion(mockID, providerVersion string) {
	keyDataMockRegistryMu.Lock()
	defer keyDataMockRegistryMu.Unlock()

	entry, ok := keyDataMockRegistry[mockID]
	if !ok {
		return
	}

	filtered := entry.versions[:0]
	for _, v := range entry.versions {
		if v.ProviderVersion != providerVersion {
			filtered = append(filtered, v)
		}
	}
	entry.versions = filtered
}

func (m *KeyDataMock) getEntry() (*keyDataMockEntry, error) {
	keyDataMockRegistryMu.RLock()
	defer keyDataMockRegistryMu.RUnlock()

	entry, ok := keyDataMockRegistry[m.MockID]
	if !ok {
		return nil, fmt.Errorf("mock key data %q not found in registry", m.MockID)
	}
	return entry, nil
}

func (m *KeyDataMock) GetCurrentVersion(_ context.Context) (KeyVersionInfo, error) {
	entry, err := m.getEntry()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	for _, v := range entry.versions {
		if v.IsCurrent {
			return v, nil
		}
	}
	return KeyVersionInfo{}, fmt.Errorf("no current version for mock key data %q", m.MockID)
}

func (m *KeyDataMock) GetVersion(_ context.Context, version string) (KeyVersionInfo, error) {
	entry, err := m.getEntry()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	for _, v := range entry.versions {
		if v.ProviderVersion == version {
			return v, nil
		}
	}
	return KeyVersionInfo{}, fmt.Errorf("version %q not found for mock key data %q", version, m.MockID)
}

func (m *KeyDataMock) ListVersions(_ context.Context) ([]KeyVersionInfo, error) {
	entry, err := m.getEntry()
	if err != nil {
		return nil, err
	}

	// Return a copy to prevent external mutation
	result := make([]KeyVersionInfo, len(entry.versions))
	copy(result, entry.versions)
	return result, nil
}

func (m *KeyDataMock) GetProviderType() ProviderType {
	return ProviderTypeMock
}

var _ KeyDataType = (*KeyDataMock)(nil)
