package database

import "errors"

// TODO: transition all the db functions to be consistent with this error

// ErrNotFound is returned when a database operation that is expected to find a record does not find the record.
var ErrNotFound = errors.New("record not found")

// ErrViolation is returned when a constraint in the database is violated (e.g. multiple rows with the same ID)
var ErrViolation = errors.New("database constraint violation")
