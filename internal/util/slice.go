package util

// CountBy returns the number of items in a slice match a predicate
func CountBy[T any](items []T, predicate func(T) bool) int {
	count := 0

	for _, item := range items {
		if predicate(item) {
			count++
		}
	}

	return count
}

// CountTrue counts the number of items are true in a boolean slice.
func CountTrue(items []bool) int {
	return CountBy(items, func(b bool) bool { return b })
}

// PrependStringToAll adds the string s to the beginning of each item passed,
// returning a new slice. Returns a nil slice unchanged.
func PrependStringToAll(s string, items []string) []string {
	if items == nil {
		return nil
	}

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = s + item
	}

	return result
}

// AppendStringToAll adds the string s to the end of each item passed, returning
// a new slice. Returns a nil slice unchanged.
func AppendStringToAll(s string, items []string) []string {
	if items == nil {
		return nil
	}

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item + s
	}

	return result
}
