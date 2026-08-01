package database

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite3 "github.com/mattn/go-sqlite3"
)

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

// ErrProtected is returned when an operation is attempted on a protected resource that cannot be modified in
// the requested way.
var ErrProtected = errors.New("resource is protected")

// wrapDatabaseMutationError normalizes provider-specific unique-constraint
// errors so API callers can return a stable conflict response without leaking
// SQLite or PostgreSQL details.
func wrapDatabaseMutationError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique || sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey) {
		return fmt.Errorf("%s: %w", operation, ErrDuplicate)
	}

	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
		return fmt.Errorf("%s: %w", operation, ErrDuplicate)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
