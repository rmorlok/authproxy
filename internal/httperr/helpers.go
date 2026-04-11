package httperr

import (
	"errors"
	"strings"
)

// As extracts an *Error from err using errors.As. Returns nil if not found.
func As(err error) *Error {
	var he *Error
	if errors.As(err, &he) {
		return he
	}
	return nil
}

// Contains checks if err is an *Error whose ResponseMsg or InternalErr contains s.
func Contains(err error, s string) bool {
	var he *Error
	if errors.As(err, &he) {
		if strings.Contains(he.ResponseMsg, s) {
			return true
		}
		if he.InternalErr != nil && strings.Contains(he.InternalErr.Error(), s) {
			return true
		}
	}
	return false
}

// IsStatus checks if err is an *Error with the given status code.
func IsStatus(err error, statusCode int) bool {
	var he *Error
	return errors.As(err, &he) && he.Status == statusCode
}
