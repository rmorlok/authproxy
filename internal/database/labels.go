package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
)

// Kubernetes-style label restrictions
const (
	// LabelKeyNameMaxLength is the maximum length for the name portion of a label key
	LabelKeyNameMaxLength = 63

	// LabelKeyPrefixMaxLength is the maximum length for the optional prefix portion of a label key
	LabelKeyPrefixMaxLength = 253

	// LabelValueMaxLength is the maximum length for a label value
	LabelValueMaxLength = 63

	// ApxyReservedPrefix is the reserved label-key prefix for system-managed
	// labels (implicit identifier labels and parent carry-forward labels).
	// User-supplied label keys may not begin with this prefix.
	ApxyReservedPrefix = "apxy/"

	// ApxyImplicitSegment is the segment used inside apxy/ keys to mark an
	// implicit identifier label, e.g. apxy/<rt>/-/id.
	ApxyImplicitSegment = "-"
)

var (
	// labelKeyNameRegex validates the name portion of a label key:
	// - 1-63 characters
	// - must start and end with alphanumeric
	// - may contain alphanumeric, '-', '_', '.'
	labelKeyNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)

	// labelKeyPrefixRegex validates the prefix portion of a label key (DNS subdomain):
	// - max 253 characters
	// - one or more DNS labels separated by '.'
	// - each label: starts/ends with alphanumeric, may contain alphanumeric and '-'
	labelKeyPrefixRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	// labelValueRegex validates a label value:
	// - 0-63 characters (can be empty)
	// - if non-empty: must start and end with alphanumeric, may contain alphanumeric, '-', '_', '.'
	labelValueRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?)?$|^[a-zA-Z0-9]?$`)

	// apxyPathSegmentRegex validates a single segment inside an apxy/ path.
	// A segment is either a DNS-label-like token (alphanumeric start/end, may
	// contain '-' in the middle) or the literal "-" sentinel used to mark
	// implicit identifier labels (e.g. apxy/cxr/-/id).
	apxyPathSegmentRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?|-)$`)
)

// Labels is a map of key-value pairs following Kubernetes label restrictions.
// Keys follow the format [prefix/]name where:
// - prefix (optional): valid DNS subdomain, max 253 characters
// - name (required): 1-63 characters, alphanumeric start/end, may contain '-', '_', '.'
// Values: 0-63 characters, if non-empty must start/end with alphanumeric
type Labels map[string]string

// Value implements the driver.Valuer interface for Labels
func (l Labels) Value() (driver.Value, error) {
	if len(l) == 0 {
		return nil, nil
	}

	b, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements the sql.Scanner interface for Labels
func (l *Labels) Scan(value interface{}) error {
	if value == nil {
		*l = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			*l = nil
			return nil
		}
		return json.Unmarshal([]byte(v), l)
	case []byte:
		if len(v) == 0 {
			*l = nil
			return nil
		}
		return json.Unmarshal(v, l)
	default:
		return fmt.Errorf("cannot convert %T to Labels", value)
	}
}

// ValidateLabelKey validates a single label key.
//
// Two grammars are accepted:
//
//  1. Standard Kubernetes-style key: [prefix/]name
//     - prefix (optional): valid DNS subdomain, max 253 characters
//     - name (required): 1-63 characters, must start/end with alphanumeric,
//     may contain '-', '_', '.'
//
//  2. Reserved apxy/ multi-segment key: apxy/<seg>(/<seg>)*/<name>
//     - each <seg> is a DNS-label-like token or the literal "-" sentinel
//     - <name> follows the standard name rule above
//     - total prefix portion (everything before the final '/') still capped
//     at LabelKeyPrefixMaxLength characters
//
// This function accepts apxy/ keys; user-input call sites should use
// ValidateUserLabelKey to additionally reject the reserved namespace.
func ValidateLabelKey(key string) error {
	if key == "" {
		return errors.New("label key cannot be empty")
	}

	if strings.HasPrefix(key, ApxyReservedPrefix) {
		return validateApxyLabelKey(key)
	}

	var prefix, name string
	if idx := strings.LastIndex(key, "/"); idx != -1 {
		prefix = key[:idx]
		name = key[idx+1:]

		// If there's a slash, the prefix must not be empty
		if prefix == "" {
			return errors.New("label key prefix cannot be empty when slash is present")
		}
	} else {
		name = key
	}

	// Validate prefix if present
	if prefix != "" {
		if len(prefix) > LabelKeyPrefixMaxLength {
			return fmt.Errorf("label key prefix exceeds maximum length of %d characters", LabelKeyPrefixMaxLength)
		}
		if !labelKeyPrefixRegex.MatchString(prefix) {
			return fmt.Errorf("label key prefix %q is not a valid DNS subdomain", prefix)
		}
	}

	// Validate name
	if name == "" {
		return errors.New("label key name cannot be empty")
	}
	if len(name) > LabelKeyNameMaxLength {
		return fmt.Errorf("label key name exceeds maximum length of %d characters", LabelKeyNameMaxLength)
	}
	if !labelKeyNameRegex.MatchString(name) {
		return fmt.Errorf("label key name %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", name)
	}

	return nil
}

// validateApxyLabelKey validates a key already known to start with apxy/.
// The grammar is apxy/<seg>(/<seg>)*/<name>.
func validateApxyLabelKey(key string) error {
	idx := strings.LastIndex(key, "/")
	// idx must exist because the key starts with "apxy/"; the final '/'
	// separates the (possibly multi-segment) prefix from the name.
	prefix := key[:idx]
	name := key[idx+1:]

	if len(prefix) > LabelKeyPrefixMaxLength {
		return fmt.Errorf("label key prefix exceeds maximum length of %d characters", LabelKeyPrefixMaxLength)
	}

	// Trim the leading "apxy" segment (we know it's there) and require at
	// least one further segment before the name — apxy/<rt>/...
	innerPath := strings.TrimPrefix(prefix, "apxy")
	if innerPath == "" {
		return errors.New("apxy/ label key requires at least one segment after apxy/")
	}
	// innerPath now starts with '/'.
	innerSegments := strings.Split(innerPath[1:], "/")
	for _, seg := range innerSegments {
		if seg == "" {
			return errors.New("apxy/ label key has empty path segment")
		}
		if !apxyPathSegmentRegex.MatchString(seg) {
			return fmt.Errorf("apxy/ label key segment %q must be a DNS label or the '-' sentinel", seg)
		}
	}

	if name == "" {
		return errors.New("label key name cannot be empty")
	}
	if len(name) > LabelKeyNameMaxLength {
		return fmt.Errorf("label key name exceeds maximum length of %d characters", LabelKeyNameMaxLength)
	}
	if !labelKeyNameRegex.MatchString(name) {
		return fmt.Errorf("label key name %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", name)
	}

	return nil
}

// ValidateUserLabelKey validates a label key supplied directly by an end user.
// In addition to the rules of ValidateLabelKey, it rejects any key in the
// reserved apxy/ namespace — those keys are managed by the system and may not
// be set, modified, or deleted through user-input endpoints.
func ValidateUserLabelKey(key string) error {
	if strings.HasPrefix(key, ApxyReservedPrefix) {
		return fmt.Errorf("label key %q is in the reserved %q namespace and cannot be set by users", key, ApxyReservedPrefix)
	}
	return ValidateLabelKey(key)
}

// ValidateLabelValue validates a single label value according to Kubernetes restrictions.
// - 0-63 characters (can be empty)
// - if non-empty: must start and end with alphanumeric, may contain alphanumeric, '-', '_', '.'
func ValidateLabelValue(value string) error {
	if len(value) > LabelValueMaxLength {
		return fmt.Errorf("label value exceeds maximum length of %d characters", LabelValueMaxLength)
	}

	if value != "" && !labelValueRegex.MatchString(value) {
		return fmt.Errorf("label value %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", value)
	}

	return nil
}

// ValidateLabels validates all labels in a map according to Kubernetes
// restrictions. apxy/-prefixed keys are accepted; this is the right validator
// for system-managed writes (e.g. carry-forward materialization).
func ValidateLabels(labels map[string]string) error {
	var result *multierror.Error
	for key, value := range labels {
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := ValidateLabelValue(value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}

	return result.ErrorOrNil()
}

// ValidateUserLabels validates a labels map supplied by a user. It applies
// the same key/value rules as ValidateLabels but rejects any key in the
// reserved apxy/ namespace.
func ValidateUserLabels(labels map[string]string) error {
	var result *multierror.Error
	for key, value := range labels {
		if err := ValidateUserLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := ValidateLabelValue(value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}

	return result.ErrorOrNil()
}

// ValidateUserLabelDeletionKeys validates a list of keys passed to a
// user-facing label-deletion endpoint. Keys must be well-formed and must not
// reference the reserved apxy/ namespace.
func ValidateUserLabelDeletionKeys(keys []string) error {
	var result *multierror.Error
	for _, k := range keys {
		if err := ValidateUserLabelKey(k); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", k, err))
		}
	}
	return result.ErrorOrNil()
}

// Validate validates all labels according to Kubernetes restrictions.
func (l Labels) Validate() error {
	if l == nil {
		return nil
	}

	result := &multierror.Error{}

	for key, value := range l {
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := ValidateLabelValue(value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}

	return result.ErrorOrNil()
}

// Get returns the value for a label key, and whether the key exists.
func (l Labels) Get(key string) (string, bool) {
	if l == nil {
		return "", false
	}
	v, ok := l[key]
	return v, ok
}

// Has returns true if the label key exists.
func (l Labels) Has(key string) bool {
	if l == nil {
		return false
	}
	_, ok := l[key]
	return ok
}

// Copy returns a deep copy of the labels.
func (l Labels) Copy() Labels {
	if l == nil {
		return nil
	}
	copy := make(Labels, len(l))
	for k, v := range l {
		copy[k] = v
	}
	return copy
}

// putLabelsInTableTx merges labels into an existing row's labels within a transaction.
// Reads current labels, merges new ones, writes back with updated timestamp.
// Returns the merged labels and the new updated_at time.
func (s *service) putLabelsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, newLabels map[string]string) (Labels, time.Time, error) {
	var currentLabels Labels
	err := s.sq.
		Select("labels").
		From(table).
		Where(where).
		RunWith(tx).
		QueryRow().
		Scan(&currentLabels)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}

	if currentLabels == nil {
		currentLabels = make(Labels)
	}
	for k, v := range newLabels {
		currentLabels[k] = v
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("labels", currentLabels).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to put labels in %s: %w", table, err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to put labels in %s: %w", table, err)
	}

	if affected == 0 {
		return nil, time.Time{}, fmt.Errorf("failed to put labels in %s; no rows updated", table)
	}

	return currentLabels, now, nil
}

// deleteLabelsInTableTx removes label keys from an existing row's labels within a transaction.
// Reads current labels, deletes specified keys, writes back with updated timestamp.
// Returns the remaining labels and the new updated_at time.
func (s *service) deleteLabelsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, keys []string) (Labels, time.Time, error) {
	var currentLabels Labels
	err := s.sq.
		Select("labels").
		From(table).
		Where(where).
		RunWith(tx).
		QueryRow().
		Scan(&currentLabels)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}

	if currentLabels != nil {
		for _, k := range keys {
			delete(currentLabels, k)
		}
	}

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("labels", currentLabels).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to delete labels in %s: %w", table, err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to delete labels in %s: %w", table, err)
	}

	if affected == 0 {
		return nil, time.Time{}, fmt.Errorf("failed to delete labels in %s; no rows updated", table)
	}

	return currentLabels, now, nil
}

// updateLabelsInTableTx replaces all labels on an existing row within a transaction.
// Writes the provided labels and updated timestamp.
// Returns the new updated_at time.
func (s *service) updateLabelsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, labels Labels) (time.Time, error) {
	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("labels", labels).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update labels in %s: %w", table, err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to update labels in %s: %w", table, err)
	}

	if affected == 0 {
		return time.Time{}, ErrNotFound
	}

	return now, nil
}

// ApidPrefixToLabelToken returns the label-key token associated with an apid
// prefix. It strips the trailing underscore so e.g. "cxr_" becomes "cxr" and
// "cxn_" becomes "cxn". Used to build apxy/ keys whose <rt> segment matches
// the resource's id prefix.
func ApidPrefixToLabelToken(p apid.Prefix) string {
	return strings.TrimSuffix(string(p), "_")
}

// BuildImplicitIdentifierLabels returns the two implicit identifier labels
// for a resource: apxy/<rt>/-/id and apxy/<rt>/-/ns, where <rt> is derived
// from the resource's id prefix. The id and namespacePath are stored as
// label values verbatim — callers must ensure they have already been
// validated by their respective rules.
func BuildImplicitIdentifierLabels(id apid.ID, namespacePath string) Labels {
	if id.IsNil() {
		return nil
	}
	rt := ApidPrefixToLabelToken(id.Prefix())
	if rt == "" {
		return nil
	}
	return Labels{
		fmt.Sprintf("%s%s/%s/id", ApxyReservedPrefix, rt, ApxyImplicitSegment): string(id),
		fmt.Sprintf("%s%s/%s/ns", ApxyReservedPrefix, rt, ApxyImplicitSegment): namespacePath,
	}
}

// BuildCarriedLabels takes a parent's labels and returns the carry-forward
// labels for a child of the parent.
//
//   - User labels on the parent (any key NOT starting with apxy/) are
//     re-keyed under apxy/<parentRt>/<key>.
//   - apxy/-prefixed labels on the parent are forwarded as-is so that
//     ancestors further up the chain remain visible. The child is expected
//     to merge its own self-implicit labels on top of this map (deeper
//     overrides shallower).
//
// parentRt is the resource-type token of the parent (e.g. "cxr", "cxn",
// "ns") — typically obtained via ApidPrefixToLabelToken.
func BuildCarriedLabels(parentRt string, parentLabels Labels) Labels {
	if parentRt == "" || len(parentLabels) == 0 {
		return nil
	}
	out := make(Labels, len(parentLabels))
	for k, v := range parentLabels {
		if strings.HasPrefix(k, ApxyReservedPrefix) {
			out[k] = v
			continue
		}
		out[fmt.Sprintf("%s%s/%s", ApxyReservedPrefix, parentRt, k)] = v
	}
	return out
}
