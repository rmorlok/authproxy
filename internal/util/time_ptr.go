package util

import "time"

// EqualTimePointers returns true if the two time pointers are equal. Both
// values being nil is considered equal.
func EqualTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}

// UtcTimePointer converts the passed time to UTC if present passing nil
// through.
func UtcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	normalized := value.UTC()
	return &normalized
}
