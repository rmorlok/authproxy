package config

import (
	"github.com/rmorlok/authproxy/context"
)

// KeyDataRawVal is where the key data is specified directly as bytes. This isn't used for config via file
// but can be used as way to return data in a config interface that has data already loaded.
type KeyDataRawVal struct {
	Raw []byte `json:"-" yaml:"-"`
}

func (kb *KeyDataRawVal) HasData(ctx context.Context) bool {
	return len(kb.Raw) > 0
}

func (kb *KeyDataRawVal) GetData(ctx context.Context) ([]byte, error) {
	return kb.Raw, nil
}
