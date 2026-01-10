package config

import (
	"context"
	"sync"

	"github.com/rmorlok/authproxy/internal/util"
)

type KeyDataRandomBytes struct {
	NumBytes  int `json:"num_bytes" yaml:"num_bytes"`
	bytes     []byte
	bytesOnce sync.Once
}

func (kf *KeyDataRandomBytes) HasData(ctx context.Context) bool {
	return true
}

func (kf *KeyDataRandomBytes) GetData(ctx context.Context) ([]byte, error) {
	kf.bytesOnce.Do(func() {
		numBytes := 16
		if kf.NumBytes > 0 {
			numBytes = kf.NumBytes
		}

		kf.bytes = util.MustGenerateSecureRandomKey(numBytes)
	})

	return kf.bytes, nil
}

func NewKeyDataRandomBytes() *KeyData {
	return &KeyData{InnerVal: &KeyDataRandomBytes{}}
}

var _ KeyDataType = (*KeyDataRandomBytes)(nil)
