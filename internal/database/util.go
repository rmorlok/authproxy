package database

import (
	"database/sql"
)

func (db *gormDB) transaction(fn func(tx *sql.Tx) error) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			db.logger.Error("panic in transaction; rolling back", "panic", p)
			err2 := tx.Rollback()
			if err2 != nil {
				db.logger.Error("error rolling back transaction after panic", "error", err2)
			}
			panic(p)
		} else if err != nil {
			db.logger.Error("error in transaction; rolling back", "error", err)
			err2 := tx.Rollback()
			if err2 != nil {
				db.logger.Error("error rolling back transaction after error", "error", err2)
			}
		} else {
			err = tx.Commit()
		}
	}()

	// Record error so defer can detect it
	err = fn(tx)

	return err
}
