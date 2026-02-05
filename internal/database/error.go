package database

import "errors"

// ErrNotFound is returned when a database operation that is expected to find a record does not find the record.
var ErrNotFound = errors.New("record not found")

// ErrNamespaceDoesNotExist is returned when a namespace does not exist for a resource that is attempting to
// be created in the specified namespace.
var ErrNamespaceDoesNotExist = errors.New("namespace does not exist")

// ErrDuplicate is returned when a database operation that is expected to be unique fails because a duplicate record
// already exists.
var ErrDuplicate = errors.New("duplicate record")

// ErrViolation is returned when a constraint in the database is violated (e.g. multiple rows with the same ID)
// after an operation that should have been unique.
var ErrViolation = errors.New("database constraint violation")
