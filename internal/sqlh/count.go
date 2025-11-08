package sqlh

import (
	"database/sql"
)

func Count(db *sql.DB, q string) (int, error) {
	row := db.QueryRow(q)
	count, _, err := ScanWithDefault(row, 0)
	return count, err
}

func MustCount(db *sql.DB, q string) int {
	count, err := Count(db, q)
	if err != nil {
		panic(err)
	}
	return count
}
