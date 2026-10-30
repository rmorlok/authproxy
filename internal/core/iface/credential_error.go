package iface

import "errors"

// ErrCredentialsUnavailable identifies credentials known to be absent or
// unusable, such as a missing stored key or a rejected OAuth refresh grant.
// Authentication implementations mark this at the source; a generic 401,
// unhealthy connection, or failure to load/decrypt credentials is not proof.
// The category says nothing about whether an earlier target request executed.
var ErrCredentialsUnavailable = errors.New("connection credentials unavailable")

// MarkCredentialsUnavailable adds a trusted classification without changing
// err's message, cause chain, or any existing HTTP status/response metadata.
// It returns nil for nil and leaves an already classified error unchanged.
// Only call it where credentials are known to be absent or unusable, never
// merely because credential resolution failed. The wrapped error can contain
// private diagnostics; public Tool errors must expose only the category.
func MarkCredentialsUnavailable(err error) error {
	if err == nil || errors.Is(err, ErrCredentialsUnavailable) {
		return err
	}
	return &credentialsUnavailableError{cause: err}
}

// credentialsUnavailableError preserves legacy proxy diagnostics while adding
// a category recognized with errors.Is. Its cause is always non-nil.
type credentialsUnavailableError struct {
	// cause retains the original error for internal diagnostics and HTTP mapping.
	cause error
}

// Error preserves the original diagnostic text exactly.
func (e *credentialsUnavailableError) Error() string { return e.cause.Error() }

// Unwrap preserves existing cause matching and HTTP-aware error extraction.
func (e *credentialsUnavailableError) Unwrap() error { return e.cause }

// Is adds category matching; errors.Is also follows Unwrap for other targets.
func (e *credentialsUnavailableError) Is(target error) bool {
	return target == ErrCredentialsUnavailable
}
