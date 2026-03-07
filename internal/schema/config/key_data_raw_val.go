package config

import (
	"context"
	"errors"
)

// KeyDataRawVal is where the key data is specified directly as bytes. This isn't used for config via file
// but can be used as way to return data in a config interface that has data already loaded.
type KeyDataRawVal struct {
	Raw []byte `json:"-" yaml:"-"`
}

func (kb *KeyDataRawVal) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if len(kb.Raw) == 0 {
		return KeyVersionInfo{}, errors.New("raw key data is empty")
	}
	hash := DataHash(kb.Raw)
	return KeyVersionInfo{
		Provider:        ProviderTypeRaw,
		ProviderID:      hash,
		ProviderVersion: hash,
		Data:            kb.Raw,
		IsCurrent:       true,
	}, nil
}

func (kb *KeyDataRawVal) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	return getVersionFromList(ctx, kb, version)
}

func (kb *KeyDataRawVal) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kb.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kb *KeyDataRawVal) GetProviderType() ProviderType {
	return ProviderTypeRaw
}

var _ KeyDataType = (*KeyDataRawVal)(nil)
