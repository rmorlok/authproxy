package config

import (
	"encoding/base64"
	"github.com/rmorlok/authproxy/context"
)

type StringValueBase64 struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (kb *StringValueBase64) HasData(ctx context.Context) bool {
	return len(kb.Base64) > 0
}

func (kb *StringValueBase64) GetData(ctx context.Context) (string, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(kb.Base64)
	if err != nil {
		return "", err
	}

	return string(decodedBytes), nil
}
