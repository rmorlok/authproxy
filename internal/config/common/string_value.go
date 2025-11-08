package common

import (
	"context"
	"fmt"
)

type StringValueType interface {
	Clone() StringValueType

	// HasValue checks if this value has data.
	HasValue(ctx context.Context) bool

	// GetValue retrieves the bytes of the key
	GetValue(ctx context.Context) (string, error)
}

type StringValue struct {
	InnerVal StringValueType `json:"-" yaml:"-"`
}

func (sv *StringValue) Inner() StringValueType {
	return sv.InnerVal
}

func (sv *StringValue) CloneValue() *StringValue {
	if sv.InnerVal == nil {
		return nil
	}

	return &StringValue{InnerVal: sv.InnerVal.Clone()}
}

func (sv *StringValue) Clone() StringValueType {
	return sv.CloneValue()
}

func (sv *StringValue) HasValue(ctx context.Context) bool {
	if sv.InnerVal == nil {
		return false
	}
	return sv.InnerVal.HasValue(ctx)
}

func (sv *StringValue) GetValue(ctx context.Context) (string, error) {
	if sv.InnerVal == nil {
		return "", fmt.Errorf("string value incorrectly configured")
	}
	return sv.InnerVal.GetValue(ctx)
}

var _ StringValueType = (*StringValue)(nil)
