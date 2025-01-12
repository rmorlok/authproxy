package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/context"
	"gorm.io/gorm"
	"time"
)

// UsedNonce represents a onetime use value (UUID) that has already been used in the system and cannot
// be used again. When used outside the system, nonces should also use some sort of expiry mechanism such that
// when they are used there is a known time that they must be retained until so that the list of used nonces doesn't
// grow infinitely.
type UsedNonce struct {
	ID          uuid.UUID `gorm:"primarykey"`
	RetainUntil time.Time `gorm:"index"`
	CreatedAt   time.Time
}

func (db *gormDB) HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (hasBeenUsed bool, err error) {
	err = db.gorm.Where("id = ?", nonce).First(&UsedNonce{}).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil // Nonce is not in the database, therefore has not been used
		}
		return false, err // An error occurred during the query
	}
	return true, nil // Nonce is found in the database, so it was used previously
}

func (db *gormDB) CheckNonceValidAndMarkUsed(
	ctx context.Context,
	nonce uuid.UUID,
	retainRecordUntil time.Time,
) (wasValid bool, err error) {
	err = db.gorm.Transaction(func(tx *gorm.DB) error {
		var usedNonce UsedNonce
		// Check if the nonce exists in the database
		err := tx.Where("id = ?", nonce).First(&usedNonce).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// Nonce is valid (not used), so mark it as used
				newUsedNonce := UsedNonce{
					ID:          nonce,
					RetainUntil: retainRecordUntil,
					CreatedAt:   ctx.Clock().Now(),
				}
				if createErr := tx.Create(&newUsedNonce).Error; createErr != nil {
					return createErr
				}
				wasValid = true
				return nil
			}
			// Return the error if it wasn't a "record not found" error
			return err
		}

		// Nonce already exists, thus it is invalid (was already used)
		wasValid = false
		return nil
	})

	// Return any errors that occurred during the transaction
	if err != nil {
		return false, err
	}

	return wasValid, nil
}
