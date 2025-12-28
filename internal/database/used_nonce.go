package database

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
)

const UsedNoncesTable = "used_nonces"

// UsedNonce represents a onetime use value (UUID) that has already been used in the system and cannot
// be used again. When used outside the system, nonces should also use some sort of expiry mechanism such that
// when they are used there is a known time that they must be retained until so that the list of used nonces doesn't
// grow infinitely.
type UsedNonce struct {
	Id          uuid.UUID
	RetainUntil time.Time
	CreatedAt   time.Time
}

func (n *UsedNonce) cols() []string {
	return []string{
		"id",
		"retain_until",
		"created_at",
	}
}

func (n *UsedNonce) fields() []any {
	return []any{
		&n.Id,
		&n.RetainUntil,
		&n.CreatedAt,
	}
}

func (n *UsedNonce) values() []any {
	return []any{
		n.Id,
		n.RetainUntil,
		n.CreatedAt,
	}
}

func (s *service) hasNonceBeenUsed(ctx context.Context, tx sq.BaseRunner, nonce uuid.UUID) (hasBeenUsed bool, err error) {
	var count int64
	err = s.sq.
		Select("COUNT(*)").
		From(UsedNoncesTable).
		Where(sq.Or{
			sq.Eq{"id": nonce},
		}).
		RunWith(s.db).
		QueryRow().
		Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (s *service) HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (hasBeenUsed bool, err error) {
	return s.hasNonceBeenUsed(ctx, s.db, nonce)
}

func (s *service) CheckNonceValidAndMarkUsed(
	ctx context.Context,
	nonce uuid.UUID,
	retainRecordUntil time.Time,
) (wasValid bool, err error) {

	err = s.transaction(func(tx *sql.Tx) error {
		hasBeenUsed, err := s.hasNonceBeenUsed(ctx, tx, nonce)
		if err != nil {
			wasValid = false
			return err
		}

		if hasBeenUsed {
			wasValid = false
			return nil
		}

		newUsedNonce := UsedNonce{
			Id:          nonce,
			RetainUntil: retainRecordUntil,
			CreatedAt:   apctx.GetClock(ctx).Now(),
		}
		dbResult, err := s.sq.
			Insert(UsedNoncesTable).
			Columns(newUsedNonce.cols()...).
			Values(newUsedNonce.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			wasValid = false
			return errors.Wrap(err, "failed to mark nonce as used")

		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			wasValid = false
			return errors.Wrap(err, "failed to mark nonce as used")
		}

		if affected != 1 {
			wasValid = false
			return errors.New("failed to mark nonce as used; no rows inserted")
		}

		wasValid = true
		return nil
	})

	// Return any errors that occurred during the transaction
	if err != nil {
		return false, err
	}

	return wasValid, nil
}
