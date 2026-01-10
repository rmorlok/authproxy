package config

import "context"

type KeyDataValue struct {
	Value string `json:"value" yaml:"value"`
}

func (kv *KeyDataValue) HasData(ctx context.Context) bool {
	return len(kv.Value) > 0
}

func (kv *KeyDataValue) GetData(ctx context.Context) ([]byte, error) {
	return []byte(kv.Value), nil
}

var _ KeyDataType = (*KeyDataValue)(nil)
