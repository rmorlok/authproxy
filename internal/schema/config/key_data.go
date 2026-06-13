package config

import (
	"context"
	"errors"
	"fmt"
)

type KeyDataType interface {
	// GetCurrentVersion retrieves the current version info including the key bytes.
	GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error)

	// GetVersion retrieves a specific version by its provider version identifier.
	GetVersion(ctx context.Context, version string) (KeyVersionInfo, error)

	// ListVersions returns all known versions from this provider. For most providers
	// this is a single-element slice containing the current version.
	ListVersions(ctx context.Context) ([]KeyVersionInfo, error)

	// GetProviderType returns the provider type identifier for this key data source.
	GetProviderType() ProviderType
}

// DataEncryptionKeyInfo is the database-visible metadata for a persisted DEK.
// KMS-style providers use these rows to unwrap DEKs and expose them as
// application-facing encryption key versions.
type DataEncryptionKeyInfo struct {
	ID              string
	EncryptionKeyID string
	Provider        ProviderType
	ProviderID      string
	ProviderVersion string
	ProtectedData   *KeyVersionProtectedData
	IsCurrent       bool
}

// KeyDataTypeWithDataEncryptionKeys is implemented by providers that resolve
// key bytes from persisted DEK rows instead of directly listing key bytes from
// the provider.
type KeyDataTypeWithDataEncryptionKeys interface {
	ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error)
}

type KeyData struct {
	InnerVal KeyDataType `json:"-" yaml:"-"`
}

func (kd *KeyData) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return KeyVersionInfo{}, errors.New("key data is nil")
	}

	return kd.InnerVal.GetCurrentVersion(ctx)
}

func (kd *KeyData) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return KeyVersionInfo{}, errors.New("key data is nil")
	}

	return kd.InnerVal.GetVersion(ctx, version)
}

func (kd *KeyData) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	return kd.InnerVal.ListVersions(ctx)
}

func (kd *KeyData) ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	if withDEKs, ok := kd.InnerVal.(KeyDataTypeWithDataEncryptionKeys); ok {
		return withDEKs.ListVersionsWithDataEncryptionKeys(ctx, deks)
	}

	return kd.InnerVal.ListVersions(ctx)
}

func (kd *KeyData) GetProviderType() ProviderType {
	if kd == nil || kd.InnerVal == nil {
		return ""
	}

	return kd.InnerVal.GetProviderType()
}

// getVersionFromList is a helper for implementations that searches ListVersions for a matching version.
func getVersionFromList(ctx context.Context, kdt KeyDataType, version string) (KeyVersionInfo, error) {
	versions, err := kdt.ListVersions(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	for _, v := range versions {
		if v.ProviderVersion == version {
			return v, nil
		}
	}

	return KeyVersionInfo{}, fmt.Errorf("version %q not found", version)
}

var _ KeyDataType = (*KeyData)(nil)
