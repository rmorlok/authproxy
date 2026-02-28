package apblob

import "errors"

// ErrBlobNotFound is returned when a requested blob does not exist.
var ErrBlobNotFound = errors.New("blob not found")
