package common

import (
	"context"
	"encoding/base64"
)

type StringValueBase64 struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (kb *StringValueBase64) HasValue(ctx context.Context) bool {
	return len(kb.Base64) > 0
}

func (kb *StringValueBase64) GetValue(ctx context.Context) (string, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(kb.Base64)
	if err != nil {
		return "", err
	}

	return string(decodedBytes), nil
}

func (kb *StringValueBase64) Clone() StringValue {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}
