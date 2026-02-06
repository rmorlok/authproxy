package sqlh

import (
	"database/sql"

	"github.com/pkg/errors"
)

// RowScanner is the interface that wraps the Scan method.
//
// Scan behaves like database/sql.Row.Scan.
type RowScanner interface {
	Scan(...interface{}) error
}

// ScanWithDefault scans a value from a database row into the provided type T.
// If the row exists, it returns the scanned value, nil error, and false (indicating default was not used).
// If no rows are found (sql.ErrNoRows), it returns the provided defaultValue, nil error, and true (indicating default was used).
// For any other scanning error, it returns the zero value of type T, the error, and false.
//
// Parameters:
//   - row: A pointer to sql.Row to scan from
//   - defaultValue: The value to return if no rows are found
//
// Returns:
//   - T: The scanned value or defaultValue if no rows
//   - error: Any error that occurred during scanning, except sql.ErrNoRows
//   - bool: True if no rows were found and the default value was used, false otherwise
func ScanWithDefault[T any](row RowScanner, defaultValue T) (T, bool, error) {
	var result T
	err := row.Scan(&result)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultValue, true, nil
		}
		return result, false, err
	}
	return result, false, nil
}
