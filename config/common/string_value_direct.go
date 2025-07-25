package common

import (
	"context"
)

// StringValueDirect is where the string data is specified directly. This isn't used for config via file
// but can be used as way to return data in a config interface that has data already loaded.
type StringValueDirect struct {
	Value string `json:"value" yaml:"value"`
}

func (kb *StringValueDirect) HasValue(ctx context.Context) bool {
	return len(kb.Value) > 0
}

func (kb *StringValueDirect) GetValue(ctx context.Context) (string, error) {
	return kb.Value, nil
}

func (kb *StringValueDirect) Clone() StringValue {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}
