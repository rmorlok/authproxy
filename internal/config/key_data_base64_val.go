package config

import (
	"context"
	"encoding/base64"
)

type KeyDataBase64Val struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (kb *KeyDataBase64Val) HasData(ctx context.Context) bool {
	return len(kb.Base64) > 0
}

func (kb *KeyDataBase64Val) GetData(ctx context.Context) ([]byte, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(kb.Base64)
	if err != nil {
		return nil, err
	}

	return decodedBytes, nil
}
