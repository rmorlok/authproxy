package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const DataEncryptionKeysTable = "data_encryption_keys"

type DataEncryptionKeyProviderMetadata map[string]string

func (m DataEncryptionKeyProviderMetadata) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data encryption key provider metadata: %w", err)
	}
	return string(b), nil
}

func (m *DataEncryptionKeyProviderMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		if v == "" {
			*m = nil
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			*m = nil
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into DataEncryptionKeyProviderMetadata", value)
	}

	return json.Unmarshal(data, m)
}

type DataEncryptionKey struct {
	Id               apid.ID
	KeyId            apid.ID
	Provider         string
	ProviderID       string
	ProviderVersion  string
	ProviderMetadata DataEncryptionKeyProviderMetadata
	ProtectedData    *sconfig.KeyVersionProtectedData
	IsCurrent        bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func (d *DataEncryptionKey) cols() []string {
	return []string{
		"id",
		"key_id",
		"provider",
		"provider_id",
		"provider_version",
		"provider_metadata",
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
		&d.KeyId,
		&d.Provider,
		&d.ProviderID,
		&d.ProviderVersion,
		&d.ProviderMetadata,
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
		d.KeyId,
		d.Provider,
		d.ProviderID,
		d.ProviderVersion,
		d.ProviderMetadata,
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

	if d.KeyId.IsNil() {
		result = multierror.Append(result, errors.New("encryption key id is required"))
	} else if err := d.KeyId.ValidatePrefix(apid.PrefixKey); err != nil {
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
					"key_id":     dek.KeyId,
					"deleted_at": nil,
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

func (s *service) GetCurrentDataEncryptionKeyForKey(ctx context.Context, keyId apid.ID) (*DataEncryptionKey, error) {
	var result DataEncryptionKey
	err := s.sq.
		Select(result.cols()...).
		From(DataEncryptionKeysTable).
		Where(sq.Eq{
			"key_id":     keyId,
			"is_current": true,
			"deleted_at": nil,
		}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get current data encryption key: %w", err)
	}

	return &result, nil
}

func (s *service) UpdateDataEncryptionKeyWrapping(ctx context.Context, dek *DataEncryptionKey) error {
	if dek == nil {
		return errors.New("data encryption key is nil")
	}
	if dek.Id.IsNil() {
		return errors.New("id is required")
	}
	if err := dek.Id.ValidatePrefix(apid.PrefixDataEncryptionKey); err != nil {
		return err
	}
	if dek.Provider == "" {
		return errors.New("provider is required")
	}
	if dek.ProviderID == "" {
		return errors.New("provider id is required")
	}
	if dek.ProviderVersion == "" {
		return errors.New("provider version is required")
	}
	if dek.ProtectedData == nil || dek.ProtectedData.IsZero() {
		return errors.New("protected data is required")
	}

	now := apctx.GetClock(ctx).Now()
	result, err := s.sq.
		Update(DataEncryptionKeysTable).
		Set("provider", dek.Provider).
		Set("provider_id", dek.ProviderID).
		Set("provider_version", dek.ProviderVersion).
		Set("provider_metadata", dek.ProviderMetadata).
		Set("protected_data", dek.ProtectedData).
		Set("updated_at", now).
		Where(sq.Eq{
			"id":         dek.Id,
			"deleted_at": nil,
		}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to update data encryption key wrapping: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update data encryption key wrapping: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	dek.UpdatedAt = now
	return nil
}

func (s *service) ClearCurrentDataEncryptionKeyFlagForKey(ctx context.Context, keyId apid.ID) error {
	now := apctx.GetClock(ctx).Now()
	_, err := s.sq.
		Update(DataEncryptionKeysTable).
		Set("is_current", false).
		Set("updated_at", now).
		Where(sq.Eq{
			"key_id":     keyId,
			"is_current": true,
			"deleted_at": nil,
		}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to clear current data encryption key flag for key %s: %w", keyId, err)
	}

	return nil
}

func (s *service) SetDataEncryptionKeyCurrentFlag(ctx context.Context, id apid.ID, isCurrent bool) error {
	now := apctx.GetClock(ctx).Now()

	return s.transaction(func(tx *sql.Tx) error {
		var keyId apid.ID
		err := s.sq.
			Select("key_id").
			From(DataEncryptionKeysTable).
			Where(sq.Eq{
				"id":         id,
				"deleted_at": nil,
			}).
			RunWith(tx).
			QueryRow().
			Scan(&keyId)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("failed to get data encryption key: %w", err)
		}

		if isCurrent {
			if _, err := s.sq.
				Update(DataEncryptionKeysTable).
				Set("is_current", false).
				Set("updated_at", now).
				Where(sq.Eq{
					"key_id":     keyId,
					"deleted_at": nil,
				}).
				RunWith(tx).
				Exec(); err != nil {
				return fmt.Errorf("failed to clear current data encryption keys: %w", err)
			}
		}

		result, err := s.sq.
			Update(DataEncryptionKeysTable).
			Set("is_current", isCurrent).
			Set("updated_at", now).
			Where(sq.Eq{
				"id":         id,
				"deleted_at": nil,
			}).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to set data encryption key current flag: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to set data encryption key current flag: %w", err)
		}
		if affected == 0 {
			return ErrNotFound
		}

		return nil
	})
}

func (s *service) ListDataEncryptionKeysForKey(ctx context.Context, keyId apid.ID) ([]*DataEncryptionKey, error) {
	var result DataEncryptionKey
	rows, err := s.sq.
		Select(result.cols()...).
		From(DataEncryptionKeysTable).
		Where(sq.Eq{
			"key_id":     keyId,
			"deleted_at": nil,
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

func (s *service) EnumerateDataEncryptionKeysForKey(
	ctx context.Context,
	keyId apid.ID,
	callback func(deks []*DataEncryptionKey, lastPage bool) (keepGoing pagination.KeepGoing, err error),
) error {
	const pageSize = 100
	offset := uint64(0)

	for {
		rows, err := s.sq.
			Select(util.ToPtr(DataEncryptionKey{}).cols()...).
			From(DataEncryptionKeysTable).
			Where(sq.Eq{
				"key_id":     keyId,
				"deleted_at": nil,
			}).
			OrderBy("created_at ASC", "id ASC").
			Limit(pageSize + 1).
			Offset(offset).
			RunWith(s.db).
			Query()
		if err != nil {
			return fmt.Errorf("failed to enumerate data encryption keys: %w", err)
		}

		var results []*DataEncryptionKey
		for rows.Next() {
			var dek DataEncryptionKey
			if err := rows.Scan(dek.fields()...); err != nil {
				rows.Close()
				return fmt.Errorf("failed to scan data encryption key: %w", err)
			}
			results = append(results, &dek)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("failed to enumerate data encryption keys: %w", err)
		}
		rows.Close()

		lastPage := len(results) <= pageSize
		if len(results) > pageSize {
			results = results[:pageSize]
		}

		keepGoing, err := callback(results, lastPage)
		if err != nil {
			return err
		}
		if keepGoing == pagination.Stop || lastPage {
			break
		}

		offset += pageSize
	}

	return nil
}
