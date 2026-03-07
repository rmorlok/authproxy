package config

import (
	"context"
	"errors"
)

type KeyDataType interface {
	// GetCurrentVersion retrieves the current version info including the key bytes.
	GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error)

	// ListVersions returns all known versions from this provider. For most providers
	// this is a single-element slice containing the current version.
	ListVersions(ctx context.Context) ([]KeyVersionInfo, error)

	// GetProviderType returns the provider type identifier for this key data source.
	GetProviderType() ProviderType
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

func (kd *KeyData) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	return kd.InnerVal.ListVersions(ctx)
}

func (kd *KeyData) GetProviderType() ProviderType {
	if kd == nil || kd.InnerVal == nil {
		return ""
	}

	return kd.InnerVal.GetProviderType()
}

var _ KeyDataType = (*KeyData)(nil)
