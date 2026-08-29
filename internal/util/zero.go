package util

import "reflect"

// IsZeroValue returns true if the given value is a zero value. This
// includes nil values and values that are zeroed out.
func IsZeroValue(value any) bool {
	if value == nil {
		return true
	}

	return reflect.ValueOf(value).IsZero()
}
