package common

import (
	"context"
	"encoding/json"
	"fmt"
)

// StringValueDirect is where the string data is specified directly. This isn't used for config via file
// but can be used as way to return data in a config interface that has data already loaded.
type StringValueDirect struct {
	Value string `json:"value" yaml:"value"`

	// IsDirectString implied how this value was loaded from the config. If true, implies this was loaded
	// as a string value instead of an object with the `direct` key. This drives how we render to JSON/YAML
	// to be consistent on the round trip.
	//
	// This field is exposed publicly to allow for testing, but should not be manipulated directly.
	IsDirectString bool `json:"-" yaml:"-"`
}

func (kb *StringValueDirect) HasValue(ctx context.Context) bool {
	return len(kb.Value) > 0
}

func (kb *StringValueDirect) GetValue(ctx context.Context) (string, error) {
	return kb.Value, nil
}

func (kb *StringValueDirect) Clone() StringValueType {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}

// MarshalJSON provides custom serialization of the object to account for if this was an inline-string or
// a nested object.
func (kb StringValueDirect) MarshalJSON() ([]byte, error) {
	if kb.IsDirectString {
		return []byte(fmt.Sprintf("\"%s\"", kb.Value)), nil
	}

	// Avoid recursive calls to this method
	type Alias StringValueDirect

	return json.Marshal(Alias(kb))
}

// MarshalYAML provides custom serialization of the object to account for if this was an inline-string or
// a nested object.
func (kb StringValueDirect) MarshalYAML() (interface{}, error) {
	if kb.IsDirectString {
		return kb.Value, nil
	}

	return map[string]string{
		"value": kb.Value,
	}, nil
}

func NewStringValueDirect(value string) *StringValue {
	return &StringValue{&StringValueDirect{
		Value:          value,
		IsDirectString: false,
	}}
}

func NewStringValueDirectInline(value string) *StringValue {
	return &StringValue{&StringValueDirect{
		Value:          value,
		IsDirectString: true,
	}}
}

var _ StringValueType = (*StringValueDirect)(nil)
