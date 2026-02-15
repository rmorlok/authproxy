package common

import (
	"context"
	"fmt"
)

type IntegerValueType interface {
	Clone() IntegerValueType

	// HasValue checks if this value has data.
	HasValue(ctx context.Context) bool

	// GetValue retrieves the value
	GetValue(ctx context.Context) (int64, error)

	// GetUint64Value retrieves the value cast as uin64
	GetUint64Value(ctx context.Context) (uint64, error)
}

type IntegerValue struct {
	InnerVal IntegerValueType `json:"-" yaml:"-"`
}

func (sv *IntegerValue) Inner() IntegerValueType {
	return sv.InnerVal
}

func (sv *IntegerValue) CloneValue() *IntegerValue {
	if sv.InnerVal == nil {
		return nil
	}

	return &IntegerValue{InnerVal: sv.InnerVal.Clone()}
}

func (sv *IntegerValue) Clone() IntegerValueType {
	return sv.CloneValue()
}

func (sv *IntegerValue) HasValue(ctx context.Context) bool {
	if sv.InnerVal == nil {
		return false
	}
	return sv.InnerVal.HasValue(ctx)
}

func (sv *IntegerValue) GetValue(ctx context.Context) (int64, error) {
	if sv.InnerVal == nil {
		return 0, fmt.Errorf("integer value incorrectly configured")
	}
	return sv.InnerVal.GetValue(ctx)
}

func (sv *IntegerValue) GetUint64Value(ctx context.Context) (uint64, error) {
	val, err := sv.GetValue(ctx)
	return uint64(val), err
}

var _ IntegerValueType = (*IntegerValue)(nil)
