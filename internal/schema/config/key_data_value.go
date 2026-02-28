package config

import (
	"context"
	"errors"
)

type KeyDataValue struct {
	Value string `json:"value" yaml:"value"`
}

func (kv *KeyDataValue) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if len(kv.Value) == 0 {
		return KeyVersionInfo{}, errors.New("value key data is empty")
	}
	data := []byte(kv.Value)
	hash := DataHash(data)
	return KeyVersionInfo{
		Provider:        ProviderTypeValue,
		ProviderID:      hash,
		ProviderVersion: hash,
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kv *KeyDataValue) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	return getVersionFromList(ctx, kv, version)
}

func (kv *KeyDataValue) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kv.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kv *KeyDataValue) GetProviderType() ProviderType {
	return ProviderTypeValue
}

var _ KeyDataType = (*KeyDataValue)(nil)
