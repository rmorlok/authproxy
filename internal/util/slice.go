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
