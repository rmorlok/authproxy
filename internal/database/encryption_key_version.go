package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/util"
)

const EncryptionKeyVersionsTable = "encryption_key_versions"

type EncryptionKeyVersion struct {
	Id              apid.ID
	EncryptionKeyId apid.ID
	Provider        string
	ProviderID      string
	ProviderVersion string
	OrderedVersion  int64
	IsCurrent       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

func (e *EncryptionKeyVersion) cols() []string {
	return []string{
		"id",
		"encryption_key_id",
		"provider",
		"provider_id",
		"provider_version",
		"ordered_version",
		"is_current",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (e *EncryptionKeyVersion) fields() []any {
	return []any{
		&e.Id,
		&e.EncryptionKeyId,
		&e.Provider,
		&e.ProviderID,
		&e.ProviderVersion,
		&e.OrderedVersion,
		&e.IsCurrent,
		&e.CreatedAt,
		&e.UpdatedAt,
		&e.DeletedAt,
	}
}

func (e *EncryptionKeyVersion) values() []any {
	return []any{
		e.Id,
		e.EncryptionKeyId,
		e.Provider,
		e.ProviderID,
		e.ProviderVersion,
		e.OrderedVersion,
		e.IsCurrent,
		e.CreatedAt,
		e.UpdatedAt,
		e.DeletedAt,
	}
}

func (ekv *EncryptionKeyVersion) Validate() error {
	result := &multierror.Error{}

	if ekv.Id.IsNil() {
		result = multierror.Append(result, errors.New("id is required"))
	} else if err := ekv.Id.ValidatePrefix(apid.PrefixEncryptionKeyVersion); err != nil {
		result = multierror.Append(result, err)
	}

	if ekv.EncryptionKeyId.IsNil() {
		result = multierror.Append(result, errors.New("encryption key id is required"))
	} else if err := ekv.EncryptionKeyId.ValidatePrefix(apid.PrefixEncryptionKey); err != nil {
		result = multierror.Append(result, err)
	}

	if ekv.Provider == "" {
		result = multierror.Append(result, errors.New("provider is required"))
	}

	if ekv.ProviderID == "" {
		result = multierror.Append(result, errors.New("provider id is required"))
	}

	if ekv.ProviderVersion == "" {
		result = multierror.Append(result, errors.New("provider version is required"))
	}

	return result.ErrorOrNil()
}

func (s *service) CreateEncryptionKeyVersion(ctx context.Context, ekv *EncryptionKeyVersion) error {
	if ekv == nil {
		return errors.New("encryption key version is nil")
	}

	if ekv.Id.IsNil() {
		ekv.Id = apid.New(apid.PrefixEncryptionKeyVersion)
	}

	if err := ekv.Validate(); err != nil {
		return err
	}

	now := apctx.GetClock(ctx).Now()
	ekv.CreatedAt = now
	ekv.UpdatedAt = now

	_, err := s.sq.
		Insert(EncryptionKeyVersionsTable).
		Columns(ekv.cols()...).
		Values(ekv.values()...).
		RunWith(s.db).
		Exec()

	if err != nil {
		return errors.Wrap(err, "failed to create encryption key version")
	}

	return nil
}

func (s *service) GetEncryptionKeyVersion(ctx context.Context, id apid.ID) (*EncryptionKeyVersion, error) {
	var result EncryptionKeyVersion

	err := s.sq.
		Select(result.cols()...).
		From(EncryptionKeyVersionsTable).
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
		return nil, errors.Wrap(err, "failed to get encryption key version")
	}

	return &result, nil
}

// getEncryptionKeyIdForNamespace looks up a namespace by path and returns its encryption_key_id.
// Returns (nil, nil) if the namespace exists but has no encryption_key_id set.
// Returns (nil, ErrNotFound) if the namespace does not exist.
func (s *service) getEncryptionKeyIdForNamespace(ctx context.Context, namespacePath string) (*apid.ID, error) {
	ns, err := s.GetNamespace(ctx, namespacePath)
	if err != nil {
		return nil, err
	}
	return ns.EncryptionKeyId, nil
}

// ForEncryptionKey methods - direct queries by encryption_key_id

func (s *service) GetCurrentEncryptionKeyVersionForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) (*EncryptionKeyVersion, error) {
	var result EncryptionKeyVersion

	err := s.sq.
		Select(result.cols()...).
		From(EncryptionKeyVersionsTable).
		Where(sq.Eq{
			"encryption_key_id": encryptionKeyId,
			"is_current":        true,
			"deleted_at":        nil,
		}).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to get current encryption key version for encryption key")
	}

	return &result, nil
}

func (s *service) ListEncryptionKeyVersionsForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) ([]*EncryptionKeyVersion, error) {
	var result EncryptionKeyVersion

	rows, err := s.sq.
		Select(result.cols()...).
		From(EncryptionKeyVersionsTable).
		Where(sq.Eq{
			"encryption_key_id": encryptionKeyId,
			"deleted_at":        nil,
		}).
		OrderBy("ordered_version ASC").
		RunWith(s.db).
		Query()

	if err != nil {
		return nil, errors.Wrap(err, "failed to list encryption key versions")
	}
	defer rows.Close()

	var results []*EncryptionKeyVersion
	for rows.Next() {
		var ekv EncryptionKeyVersion
		if err := rows.Scan(ekv.fields()...); err != nil {
			return nil, errors.Wrap(err, "failed to scan encryption key version")
		}
		results = append(results, &ekv)
	}

	return results, rows.Err()
}

func (s *service) GetMaxOrderedVersionForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) (int64, error) {
	var maxVersion sql.NullInt64

	err := s.sq.
		Select("MAX(ordered_version)").
		From(EncryptionKeyVersionsTable).
		Where(sq.Eq{
			"encryption_key_id": encryptionKeyId,
			"deleted_at":        nil,
		}).
		RunWith(s.db).
		QueryRow().
		Scan(&maxVersion)

	if err != nil {
		return 0, errors.Wrap(err, "failed to get max ordered version")
	}

	if !maxVersion.Valid {
		return 0, nil
	}

	return maxVersion.Int64, nil
}

func (s *service) ClearCurrentFlagForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) error {
	now := apctx.GetClock(ctx).Now()

	_, err := s.sq.
		Update(EncryptionKeyVersionsTable).
		Set("is_current", false).
		Set("updated_at", now).
		Where(sq.Eq{
			"encryption_key_id": encryptionKeyId,
			"is_current":        true,
			"deleted_at":        nil,
		}).
		RunWith(s.db).
		Exec()

	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to clear current flag for encryption key %s", encryptionKeyId))
	}

	return nil
}

// ForNamespace methods - resolve namespace path to encryption_key_id, then delegate

func (s *service) GetCurrentEncryptionKeyVersionForNamespace(ctx context.Context, namespacePath string) (*EncryptionKeyVersion, error) {
	ekId, err := s.getEncryptionKeyIdForNamespace(ctx, namespacePath)
	if err != nil {
		return nil, err
	}
	if ekId == nil {
		return nil, nil
	}
	return s.GetCurrentEncryptionKeyVersionForEncryptionKey(ctx, *ekId)
}

func (s *service) ListEncryptionKeyVersionsForNamespace(ctx context.Context, namespacePath string) ([]*EncryptionKeyVersion, error) {
	ekId, err := s.getEncryptionKeyIdForNamespace(ctx, namespacePath)
	if err != nil {
		return nil, err
	}
	if ekId == nil {
		return nil, nil
	}
	return s.ListEncryptionKeyVersionsForEncryptionKey(ctx, *ekId)
}

func (s *service) GetMaxOrderedVersionForNamespace(ctx context.Context, namespacePath string) (int64, error) {
	ekId, err := s.getEncryptionKeyIdForNamespace(ctx, namespacePath)
	if err != nil {
		return 0, err
	}
	if ekId == nil {
		return 0, nil
	}
	return s.GetMaxOrderedVersionForEncryptionKey(ctx, *ekId)
}

func (s *service) ClearCurrentFlagForNamespace(ctx context.Context, namespacePath string) error {
	ekId, err := s.getEncryptionKeyIdForNamespace(ctx, namespacePath)
	if err != nil {
		return err
	}
	if ekId == nil {
		return nil
	}
	return s.ClearCurrentFlagForEncryptionKey(ctx, *ekId)
}

func (s *service) DeleteEncryptionKeyVersion(ctx context.Context, id apid.ID) error {
	result, err := s.sq.
		Update(EncryptionKeyVersionsTable).
		Set("deleted_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		Exec()

	if err != nil {
		return errors.Wrap(err, "failed to delete encryption key version")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get rows affected")
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *service) DeleteEncryptionKeyVersionsForEncryptionKey(ctx context.Context, encryptionKeyId apid.ID) error {
	_, err := s.sq.
		Update(EncryptionKeyVersionsTable).
		Set("deleted_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{"encryption_key_id": encryptionKeyId, "deleted_at": nil}).
		RunWith(s.db).
		Exec()

	if err != nil {
		return errors.Wrap(err, "failed to delete encryption key versions for encryption key")
	}

	return nil
}

func (s *service) SetEncryptionKeyVersionCurrentFlag(ctx context.Context, id apid.ID, isCurrent bool) error {
	now := apctx.GetClock(ctx).Now()

	result, err := s.sq.
		Update(EncryptionKeyVersionsTable).
		Set("is_current", isCurrent).
		Set("updated_at", now).
		Where(sq.Eq{
			"id":         id,
			"is_current": false,
			"deleted_at": nil,
		}).
		RunWith(s.db).
		Exec()

	if err != nil {
		return errors.Wrap(err, "failed to set encryption key version current flag")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get rows affected")
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *service) EnumerateEncryptionKeyVersionsForKey(
	ctx context.Context,
	ekId apid.ID,
	callback func(ekvs []*EncryptionKeyVersion, lastPage bool) (stop bool, err error),
) error {
	const pageSize = 100
	offset := uint64(0)

	for {
		rows, err := s.sq.
			Select(util.ToPtr(EncryptionKeyVersion{}).cols()...).
			From(EncryptionKeyVersionsTable).
			Where(sq.Eq{
				"encryption_key_id": ekId,
				"deleted_at":        nil,
			}).
			OrderBy("id").
			Limit(pageSize + 1).
			Offset(offset).
			RunWith(s.db).
			Query()
		if err != nil {
			return err
		}

		var results []*EncryptionKeyVersion
		for rows.Next() {
			var r EncryptionKeyVersion
			if err := rows.Scan(r.fields()...); err != nil {
				rows.Close()
				return err
			}
			results = append(results, &r)
		}
		rows.Close()

		lastPage := len(results) <= pageSize
		if len(results) > pageSize {
			results = results[:pageSize]
		}

		stop, err := callback(results, lastPage)
		if err != nil {
			return err
		}

		if stop || lastPage {
			break
		}

		offset += pageSize
	}

	return nil
}
