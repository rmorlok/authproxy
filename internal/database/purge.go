package database

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
)

// tablesToPurge lists all tables that use soft deletes via a deleted_at column.
var tablesToPurge = []string{
	ActorTable,
	ConnectionsTable,
	ConnectorVersionsTable,
	NamespacesTable,
	OAuth2TokensTable,
}

// PurgeSoftDeletedRecords hard-deletes all soft-deleted records where deleted_at is before olderThan.
// Deletes are performed in a single transaction across all tables.
func (s *service) PurgeSoftDeletedRecords(ctx context.Context, olderThan time.Time) (int64, error) {
	var totalDeleted int64

	err := s.transaction(func(tx *sql.Tx) error {
		for _, table := range tablesToPurge {
			query, args, err := s.sq.
				Delete(table).
				Where(sq.NotEq{"deleted_at": nil}).
				Where(sq.Lt{"deleted_at": olderThan}).
				ToSql()
			if err != nil {
				return err
			}

			result, err := tx.ExecContext(ctx, query, args...)
			if err != nil {
				return err
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return err
			}

			totalDeleted += rowsAffected
		}

		return nil
	})

	return totalDeleted, err
}
