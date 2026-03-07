package config

import (
	"context"
	"encoding/base64"
	"errors"
)

type KeyDataBase64Val struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (kb *KeyDataBase64Val) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if len(kb.Base64) == 0 {
		return KeyVersionInfo{}, errors.New("base64 key data is empty")
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(kb.Base64)
	if err != nil {
		return KeyVersionInfo{}, err
	}
	hash := DataHash(decodedBytes)
	return KeyVersionInfo{
		Provider:        ProviderTypeBase64,
		ProviderID:      hash,
		ProviderVersion: hash,
		Data:            decodedBytes,
		IsCurrent:       true,
	}, nil
}

func (kb *KeyDataBase64Val) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kb.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kb *KeyDataBase64Val) GetProviderType() ProviderType {
	return ProviderTypeBase64
}

var _ KeyDataType = (*KeyDataBase64Val)(nil)
