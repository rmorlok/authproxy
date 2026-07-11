package util

// ToPtr returns a pointer to the value.
func ToPtr[T any](v T) *T {
	return &v
}

// ToPtrNonZero returns a pointer to the value if it is not the zero value of
// its type, otherwise returns nil.
func ToPtrNonZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
