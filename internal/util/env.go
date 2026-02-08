package util

import (
	"os"
	"strings"
)

// GetEnvDefault returns the value of the environment variable with the given key, or the fallback value if the variable is not set or empty.
func GetEnvDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
