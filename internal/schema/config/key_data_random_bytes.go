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

func (kf *KeyDataRandomBytes) generateBytes() []byte {
	kf.bytesOnce.Do(func() {
		numBytes := 16
		if kf.NumBytes > 0 {
			numBytes = kf.NumBytes
		}
		kf.bytes = util.MustGenerateSecureRandomKey(numBytes)
	})
	return kf.bytes
}

func (kf *KeyDataRandomBytes) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	data := kf.generateBytes()
	hash := DataHash(data)
	return KeyVersionInfo{
		Provider:        ProviderTypeRandom,
		ProviderID:      hash,
		ProviderVersion: hash,
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kf *KeyDataRandomBytes) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kf.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kf *KeyDataRandomBytes) GetProviderType() ProviderType {
	return ProviderTypeRandom
}

func NewKeyDataRandomBytes() *KeyData {
	return &KeyData{InnerVal: &KeyDataRandomBytes{}}
}

var _ KeyDataType = (*KeyDataRandomBytes)(nil)
