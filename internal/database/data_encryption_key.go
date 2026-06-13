package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const DataEncryptionKeysTable = "data_encryption_keys"

type DataEncryptionKey struct {
	Id              apid.ID
	EncryptionKeyId apid.ID
	Provider        string
	ProviderID      string
	ProviderVersion string
	ProtectedData   *sconfig.KeyVersionProtectedData
	IsCurrent       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

func (d *DataEncryptionKey) cols() []string {
	return []string{
		"id",
		"encryption_key_id",
		"provider",
		"provider_id",
		"provider_version",
		"protected_data",
		"is_current",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (d *DataEncryptionKey) fields() []any {
	return []any{
		&d.Id,
		&d.EncryptionKeyId,
		&d.Provider,
		&d.ProviderID,
		&d.ProviderVersion,
		&d.ProtectedData,
		&d.IsCurrent,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.DeletedAt,
	}
}

func (d *DataEncryptionKey) values() []any {
	return []any{
		d.Id,
		d.EncryptionKeyId,
		d.Provider,
		d.ProviderID,
		d.ProviderVersion,
		d.ProtectedData,
		d.IsCurrent,
		d.CreatedAt,
		d.UpdatedAt,
		d.DeletedAt,
	}
}

func (d *DataEncryptionKey) Validate() error {
	result := &multierror.Error{}

	if d.Id.IsNil() {
		result = multierror.Append(result, errors.New("id is required"))
	} else if err := d.Id.ValidatePrefix(apid.PrefixDataEncryptionKey); err != nil {
		result = multierror.Append(result, err)
	}

	if d.EncryptionKeyId.IsNil() {
		result = multierror.Append(result, errors.New("encryption key id is required"))
	} else if err := d.EncryptionKeyId.ValidatePrefix(apid.PrefixEncryptionKey); err != nil {
		result = multierror.Append(result, err)
	}

	if d.Provider == "" {
		result = multierror.Append(result, errors.New("provider is required"))
	}

	if d.ProviderID == "" {
		result = multierror.Append(result, errors.New("provider id is required"))
	}

	if d.ProviderVersion == "" {
		result = multierror.Append(result, errors.New("provider version is required"))
	}

	if d.ProtectedData == nil || d.ProtectedData.IsZero() {
		result = multierror.Append(result, errors.New("protected data is required"))
	}

	return result.ErrorOrNil()
}

func (s *service) CreateDataEncryptionKey(ctx context.Context, dek *DataEncryptionKey) error {
	if dek == nil {
		return errors.New("data encryption key is nil")
	}

	if dek.Id.IsNil() {
		dek.Id = apid.New(apid.PrefixDataEncryptionKey)
	}

	if err := dek.Validate(); err != nil {
		return err
	}

	now := apctx.GetClock(ctx).Now()
	dek.CreatedAt = now
	dek.UpdatedAt = now

	return s.transaction(func(tx *sql.Tx) error {
		if dek.IsCurrent {
			if _, err := s.sq.
				Update(DataEncryptionKeysTable).
				Set("is_current", false).
				Set("updated_at", now).
				Where(sq.Eq{
					"encryption_key_id": dek.EncryptionKeyId,
					"deleted_at":        nil,
				}).
				RunWith(tx).
				Exec(); err != nil {
				return fmt.Errorf("failed to clear current data encryption keys: %w", err)
			}
		}

		_, err := s.sq.
			Insert(DataEncryptionKeysTable).
			Columns(dek.cols()...).
			Values(dek.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to create data encryption key: %w", err)
		}

		return nil
	})
}

func (s *service) GetDataEncryptionKey(ctx context.Context, id apid.ID) (*DataEncryptionKey, error) {
	var result DataEncryptionKey
	err := s.sq.
		Select(result.cols()...).
		From(DataEncryptionKeysTable).
		Where(sq.Eq{
			"id":         id,
			"deleted_at": nil,
		}).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get data encryption key: %w", err)
	}

	return &result, nil
}

func (s *service) ListDataEncryptionKeysForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) ([]*DataEncryptionKey, error) {
	var result DataEncryptionKey
	rows, err := s.sq.
		Select(result.cols()...).
		From(DataEncryptionKeysTable).
		Where(sq.Eq{
			"encryption_key_id": encryptionKeyId,
			"deleted_at":        nil,
		}).
		OrderBy("created_at ASC", "id ASC").
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, fmt.Errorf("failed to list data encryption keys: %w", err)
	}
	defer rows.Close()

	var results []*DataEncryptionKey
	for rows.Next() {
		var dek DataEncryptionKey
		if err := rows.Scan(dek.fields()...); err != nil {
			return nil, fmt.Errorf("failed to scan data encryption key: %w", err)
		}
		results = append(results, &dek)
	}

	return results, rows.Err()
}
