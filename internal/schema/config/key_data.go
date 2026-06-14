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
// Providers use these rows to unwrap DEKs and expose runtime key bytes without
// storing provider key versions in the database.
type DataEncryptionKeyInfo struct {
	ID              string
	EncryptionKeyID string
	Provider        ProviderType
	ProviderID      string
	ProviderVersion string
	ProtectedData   *KeyVersionProtectedData
	IsCurrent       bool
}

// KeyWrappingKeyInfo identifies the provider key material currently used to
// wrap DEKs. The wrapping key bytes are deliberately not exposed here.
type KeyWrappingKeyInfo struct {
	Provider        ProviderType
	ProviderID      string
	ProviderVersion string
	Metadata        map[string]string
}

// GeneratedDataEncryptionKey is protected DEK material produced by a key data
// provider. The plaintext DEK is returned for immediate cache use by callers,
// but only the provider metadata and ProtectedData should be persisted.
type GeneratedDataEncryptionKey struct {
	Provider        ProviderType
	ProviderID      string
	ProviderVersion string
	ProtectedData   KeyVersionProtectedData
	Data            []byte
}

// KeyDataRequiresDataEncryptionKeys is implemented by providers that resolve
// key bytes from persisted DEK rows instead of directly listing key bytes from
// the provider. Implementing this interface on the provider signals to the
// encryption service to do a pre-load of DEKs from the database.
type KeyDataRequiresDataEncryptionKeys interface {
	ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error)
}

// KeyDataWrapsDataEncryptionKeys is implemented by providers that manage DEK
// wrapping themselves. Providers that expose raw key bytes do not need to
// implement this; the KeyData facade can wrap and unwrap with the current
// KeyVersionInfo.Data.
type KeyDataWrapsDataEncryptionKeys interface {
	CurrentWrappingKey(ctx context.Context) (KeyWrappingKeyInfo, error)
	WrapDataEncryptionKey(ctx context.Context, dek []byte) (GeneratedDataEncryptionKey, error)
	UnwrapDataEncryptionKey(ctx context.Context, dek DataEncryptionKeyInfo) ([]byte, error)
}

// KeyDataGeneratesDataEncryptionKeys is implemented by providers that can
// natively generate and wrap a new DEK for persistence in data_encryption_keys.
// KeyData itself also exposes GenerateDataEncryptionKey, falling back to
// AuthProxy-generated random bytes plus provider wrapping when this interface
// is not implemented by the inner provider.
type KeyDataGeneratesDataEncryptionKeys interface {
	GenerateDataEncryptionKey(ctx context.Context) (GeneratedDataEncryptionKey, error)
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

// RequiresDataEncryptionKeys checks if the provider for this key data requires
// data encryption keys to be pre-loaded from the database.
func (kd *KeyData) RequiresDataEncryptionKeys() bool {
	if kd == nil || kd.InnerVal == nil {
		return false
	}

	_, ok := kd.InnerVal.(KeyDataRequiresDataEncryptionKeys)
	return ok
}

// ListVersionsWithDataEncryptionKeys allows the provider to list key version infos while supplying
// data encryption keys to it. Consumers of KeyData should use this method and supply the DEKs if
// RequiresDataEncryptionKeys() returns true. If RequiresDataEncryptionKeys() returns false, callers
// can use ListVersions(...) directly. If this is called and this provider does not require DEKs,
// it automatically returns the result of ListVersions(...).
func (kd *KeyData) ListVersionsWithDataEncryptionKeys(
	ctx context.Context,
	deks []DataEncryptionKeyInfo,
) ([]KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	if withDEKs, ok := kd.InnerVal.(KeyDataRequiresDataEncryptionKeys); ok {
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
func getVersionFromList(
	ctx context.Context,
	kdt KeyDataType,
	version string,
) (KeyVersionInfo, error) {
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
