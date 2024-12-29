package config

import (
	"github.com/rmorlok/authproxy/context"
)

// StringValueDirect is where the key data is specified directly as bytes. This isn't used for config via file
// but can be used as way to return data in a config interface that has data already loaded.
type StringValueDirect struct {
	Value string `json:"-" yaml:"-"`
}

func (kb *StringValueDirect) HasData(ctx context.Context) bool {
	return len(kb.Value) > 0
}

func (kb *StringValueDirect) GetData(ctx context.Context) (string, error) {
	return kb.Value, nil
}
