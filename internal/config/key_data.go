package config

import (
	"context"
	"errors"
)

type KeyDataType interface {
	// HasData checks if this value has data.
	HasData(ctx context.Context) bool

	// GetData retrieves the bytes of the key
	GetData(ctx context.Context) ([]byte, error)
}

type KeyData struct {
	InnerVal KeyDataType `json:"-" yaml:"-"`
}

func (kd *KeyData) HasData(ctx context.Context) bool {
	if kd == nil || kd.InnerVal == nil {
		return false
	}

	return kd.InnerVal.HasData(ctx)
}

func (kd *KeyData) GetData(ctx context.Context) ([]byte, error) {
	if kd == nil || kd.InnerVal == nil {
		return nil, errors.New("key data is nil")
	}

	return kd.InnerVal.GetData(ctx)
}

var _ KeyDataType = (*KeyData)(nil)
