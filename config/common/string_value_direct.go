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

	// IsDirect implies how this value was loaded from the config, without a nested sub-object. If true, implies
	// this was loaded as a string value instead of an object with the `value` key. This drives how we render
	// to JSON/YAML to be consistent on the round trip.
	//
	// This field is exposed publicly to allow for testing, but should not be manipulated directly.
	IsDirect bool `json:"-" yaml:"-"`

	// IsNonString implies that if IsDirect is true, the value was not a string (e.g. number/bool). This is used to
	// drive re-rendering of the data consistently to omit quotes.
	//
	// This field is exposed publicly to allow for testing, but should not be manipulated directly.
	IsNonString bool `json:"-" yaml:"-"`
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
	if kb.IsDirect {
		if kb.IsNonString {
			return []byte(kb.Value), nil
		}
		return []byte(fmt.Sprintf("\"%s\"", kb.Value)), nil
	}

	// Avoid recursive calls to this method
	type Alias StringValueDirect

	return json.Marshal(Alias(kb))
}

// MarshalYAML provides custom serialization of the object to account for if this was an inline-string or
// a nested object.
func (kb StringValueDirect) MarshalYAML() (interface{}, error) {
	if kb.IsDirect {
		return kb.Value, nil
	}

	return map[string]string{
		"value": kb.Value,
	}, nil
}

func NewStringValueDirect(value string) *StringValue {
	return &StringValue{&StringValueDirect{
		Value:    value,
		IsDirect: false,
	}}
}

func NewStringValueDirectInline(value string) *StringValue {
	return &StringValue{&StringValueDirect{
		Value:    value,
		IsDirect: true,
	}}
}

var _ StringValueType = (*StringValueDirect)(nil)
