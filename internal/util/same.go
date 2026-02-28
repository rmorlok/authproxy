package util

import "reflect"

// SameInstance reports whether a and b are the same underlying pointer instance.
// Returns false if either is nil, not a pointer, or they point to different objects.
func SameInstance(a, b any) bool {
	if a == nil || b == nil {
		return false
	}

	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)

	// If interfaces wrap pointers, compare addresses.
	if va.Kind() == reflect.Pointer && vb.Kind() == reflect.Pointer {
		return va.Pointer() == vb.Pointer()
	}

	// You can extend this for other reference kinds if you want,
	// but for "store objects" this is usually enough.
	return false
}
