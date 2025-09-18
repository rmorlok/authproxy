package util

// First2 returns the first return value from a function that returns two values
func First2[T any, U any](value T, _ U) T {
	return value
}

// First3 returns the first return value from a function that returns three values
func First3[T any, U any, V any](value T, _ U, _ V) T {
	return value
}

// Second2 returns the second return value from a function that returns two values
func Second2[T any, U any](_ T, value U) U {
	return value
}

// Second3 returns the second return value from a function that returns three values
func Second3[T any, U any, V any](_ T, value U, _ V) U {
	return value
}

// Third3 returns the third return value from a function that returns three values
func Third3[T any, U any, V any](_ T, _ U, value V) V {
	return value
}
