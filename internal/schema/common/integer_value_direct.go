package common

import (
	"context"
	"encoding/json"
	"strconv"
)

// IntegerValueDirect is where the integer data is specified directly.
type IntegerValueDirect struct {
	Value int64 `json:"value" yaml:"value"`

	// IsDirect implies how this value was loaded from the config, without a nested sub-object. If true, implies
	// this was loaded as a integer value instead of an object with the `value` key. This drives how we render
	// to JSON/YAML to be consistent on the round trip.
	//
	// This field is exposed publicly to allow for testing, but should not be manipulated directly.
	IsDirect bool `json:"-" yaml:"-"`
}

func (kb *IntegerValueDirect) HasValue(ctx context.Context) bool {
	return true
}

func (kb *IntegerValueDirect) GetValue(ctx context.Context) (int64, error) {
	return kb.Value, nil
}

func (kb *IntegerValueDirect) GetUint64Value(ctx context.Context) (uint64, error) {
	return uint64(kb.Value), nil
}

func (kb *IntegerValueDirect) Clone() IntegerValueType {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}

// MarshalJSON provides custom serialization of the object to account for if this was an inline-integer or
// a nested object.
func (kb IntegerValueDirect) MarshalJSON() ([]byte, error) {
	if kb.IsDirect {
		return []byte(strconv.FormatInt(kb.Value, 10)), nil
	}

	// Avoid recursive calls to this method
	type Alias IntegerValueDirect

	return json.Marshal(Alias(kb))
}

// MarshalYAML provides custom serialization of the object to account for if this was an inline-integer or
// a nested object.
func (kb IntegerValueDirect) MarshalYAML() (interface{}, error) {
	if kb.IsDirect {
		return kb.Value, nil
	}

	return map[string]int64{
		"value": kb.Value,
	}, nil
}

func NewIntegerValueDirect(value int64) *IntegerValue {
	return &IntegerValue{&IntegerValueDirect{
		Value:    value,
		IsDirect: false,
	}}
}

func NewIntegerValueDirectInline(value int64) *IntegerValue {
	return &IntegerValue{&IntegerValueDirect{
		Value:    value,
		IsDirect: true,
	}}
}

var _ IntegerValueType = (*IntegerValueDirect)(nil)
