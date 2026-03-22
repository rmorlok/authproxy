package client

import "fmt"

// APIError represents an error response from the AuthProxy admin API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("authproxy API error (HTTP %d): %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error is a 404.
func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == 404
	}
	return false
}
