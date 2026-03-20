package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
)

const (
	// AnnotationsTotalMaxSize is the maximum total size of all annotations (keys + values) in bytes.
	AnnotationsTotalMaxSize = 256 * 1024 // 256KB
)

// Annotations is a map of key-value pairs similar to Kubernetes annotations.
// Keys follow the same format as label keys ([prefix/]name).
// Values have no format restriction — any string is allowed.
// Total size of all annotations (keys + values) must not exceed 256KB.
type Annotations map[string]string

// Value implements the driver.Valuer interface for Annotations
func (a Annotations) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}

	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements the sql.Scanner interface for Annotations
func (a *Annotations) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			*a = nil
			return nil
		}
		return json.Unmarshal([]byte(v), a)
	case []byte:
		if len(v) == 0 {
			*a = nil
			return nil
		}
		return json.Unmarshal(v, a)
	default:
		return fmt.Errorf("cannot convert %T to Annotations", value)
	}
}

// ValidateAnnotationKey validates a single annotation key.
// Annotation keys follow the same format as label keys.
func ValidateAnnotationKey(key string) error {
	return ValidateLabelKey(key)
}

// ValidateAnnotationValue validates a single annotation value.
// Annotation values have no format restriction — any string is allowed.
// Individual value size is not restricted; only the total annotations size is checked.
func ValidateAnnotationValue(_ string) error {
	return nil
}

// ValidateAnnotations validates all annotations in a map.
func ValidateAnnotations(annotations map[string]string) error {
	var result *multierror.Error
	totalSize := 0
	for key, value := range annotations {
		if err := ValidateAnnotationKey(key); err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "invalid annotation key %q", key))
		}
		totalSize += len(key) + len(value)
	}

	if totalSize > AnnotationsTotalMaxSize {
		result = multierror.Append(result, fmt.Errorf("total annotations size %d exceeds maximum of %d bytes", totalSize, AnnotationsTotalMaxSize))
	}

	return result.ErrorOrNil()
}

// Validate validates all annotations.
func (a Annotations) Validate() error {
	if a == nil {
		return nil
	}

	return ValidateAnnotations(a)
}

// Get returns the value for an annotation key, and whether the key exists.
func (a Annotations) Get(key string) (string, bool) {
	if a == nil {
		return "", false
	}
	v, ok := a[key]
	return v, ok
}

// Has returns true if the annotation key exists.
func (a Annotations) Has(key string) bool {
	if a == nil {
		return false
	}
	_, ok := a[key]
	return ok
}

// Copy returns a deep copy of the annotations.
func (a Annotations) Copy() Annotations {
	if a == nil {
		return nil
	}
	cpy := make(Annotations, len(a))
	for k, v := range a {
		cpy[k] = v
	}
	return cpy
}

// putAnnotationsInTableTx merges annotations into an existing row's annotations within a transaction.
// Reads current annotations, merges new ones, writes back with updated timestamp.
// Returns the merged annotations and the new updated_at time.
func (s *service) putAnnotationsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, newAnnotations map[string]string) (Annotations, time.Time, error) {
	var currentAnnotations Annotations
	err := s.sq.
		Select("annotations").
		From(table).
		Where(where).
		RunWith(tx).
		QueryRow().
		Scan(&currentAnnotations)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}

	if currentAnnotations == nil {
		currentAnnotations = make(Annotations)
	}
	for k, v := range newAnnotations {
		currentAnnotations[k] = v
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("annotations", currentAnnotations).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, time.Time{}, errors.Wrapf(err, "failed to put annotations in %s", table)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, time.Time{}, errors.Wrapf(err, "failed to put annotations in %s", table)
	}

	if affected == 0 {
		return nil, time.Time{}, fmt.Errorf("failed to put annotations in %s; no rows updated", table)
	}

	return currentAnnotations, now, nil
}

// deleteAnnotationsInTableTx removes annotation keys from an existing row's annotations within a transaction.
// Reads current annotations, deletes specified keys, writes back with updated timestamp.
// Returns the remaining annotations and the new updated_at time.
func (s *service) deleteAnnotationsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, keys []string) (Annotations, time.Time, error) {
	var currentAnnotations Annotations
	err := s.sq.
		Select("annotations").
		From(table).
		Where(where).
		RunWith(tx).
		QueryRow().
		Scan(&currentAnnotations)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}

	if currentAnnotations != nil {
		for _, k := range keys {
			delete(currentAnnotations, k)
		}
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("annotations", currentAnnotations).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, time.Time{}, errors.Wrapf(err, "failed to delete annotations in %s", table)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, time.Time{}, errors.Wrapf(err, "failed to delete annotations in %s", table)
	}

	if affected == 0 {
		return nil, time.Time{}, fmt.Errorf("failed to delete annotations in %s; no rows updated", table)
	}

	return currentAnnotations, now, nil
}

// updateAnnotationsInTableTx replaces all annotations on an existing row within a transaction.
// Writes the provided annotations and updated timestamp.
// Returns the new updated_at time.
func (s *service) updateAnnotationsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, annotations Annotations) (time.Time, error) {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("annotations", annotations).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to update annotations in %s", table)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to update annotations in %s", table)
	}

	if affected == 0 {
		return time.Time{}, ErrNotFound
	}

	return now, nil
}
