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

	// ApxyLabelValueMaxLength is the maximum length for a label value stored
	// under an apxy/-prefixed key. System-managed labels such as
	// apxy/<rt>/-/ns can hold a namespace path that may exceed the standard
	// LabelValueMaxLength. User-supplied values are still capped at
	// LabelValueMaxLength via ValidateLabelValue.
	ApxyLabelValueMaxLength = 253

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

	// apxyLabelValueRegex matches the same character classes as labelValueRegex
	// but allows up to ApxyLabelValueMaxLength characters total. Used for
	// system-managed apxy/-prefixed values such as namespace paths.
	apxyLabelValueRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,251}[a-zA-Z0-9])?)?$|^[a-zA-Z0-9]?$`)
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

// ValidateApxyLabelValue validates a label value stored under an apxy/-prefixed
// key. It allows up to ApxyLabelValueMaxLength characters so namespace paths
// (e.g. root.foo.bar.baz...) can fit. Character rules are otherwise the same
// as ValidateLabelValue.
func ValidateApxyLabelValue(value string) error {
	if len(value) > ApxyLabelValueMaxLength {
		return fmt.Errorf("apxy label value exceeds maximum length of %d characters", ApxyLabelValueMaxLength)
	}

	if value != "" && !apxyLabelValueRegex.MatchString(value) {
		return fmt.Errorf("apxy label value %q must start and end with alphanumeric and contain only alphanumeric, '-', '_', or '.'", value)
	}

	return nil
}

// ValidateLabels validates all labels in a map. apxy/-prefixed keys are
// accepted (use ValidateUserLabels at user-input boundaries instead) and
// values stored under apxy/ keys are validated against the longer
// ApxyLabelValueMaxLength cap.
func ValidateLabels(labels map[string]string) error {
	var result *multierror.Error
	for key, value := range labels {
		if err := ValidateLabelKey(key); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label key %q: %w", key, err))
		}
		if err := validateValueForKey(key, value); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid label value for key %q: %w", key, err))
		}
	}

	return result.ErrorOrNil()
}

// validateValueForKey selects the appropriate value validator based on
// whether the key is in the apxy/ namespace.
func validateValueForKey(key, value string) error {
	if strings.HasPrefix(key, ApxyReservedPrefix) {
		return ValidateApxyLabelValue(value)
	}
	return ValidateLabelValue(value)
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

// Validate validates all labels (system mode — apxy/ keys allowed, with the
// longer ApxyLabelValueMaxLength value cap for those keys).
func (l Labels) Validate() error {
	if l == nil {
		return nil
	}

	return ValidateLabels(map[string]string(l))
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

// replaceUserLabelsInTableTx replaces only the user-portion of an existing
// row's labels, preserving any apxy/-prefixed system labels. Used by
// UpdateXLabels endpoints (which expose a full-replace semantic to users
// over the user-managed portion only — system labels are untouchable from
// user input).
//
// Returns the merged final label set written and the new updated_at time.
func (s *service) replaceUserLabelsInTableTx(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, newUserLabels Labels) (Labels, time.Time, error) {
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

	_, apxy := SplitUserAndApxyLabels(currentLabels)
	merged := MergeApxyAndUserLabels(newUserLabels, apxy)

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(table).
		Set("labels", merged).
		Set("updated_at", now).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to replace user labels in %s: %w", table, err)
	}

	affected, err := dbResult.RowsAffected()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to replace user labels in %s: %w", table, err)
	}

	if affected == 0 {
		return nil, time.Time{}, ErrNotFound
	}

	return merged, now, nil
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

// NamespaceLabelToken is the <rt> token used in apxy/ keys that reference a
// namespace. Namespaces are path-keyed (not apid-keyed) so the token is
// hard-coded rather than derived from an apid prefix.
const NamespaceLabelToken = "ns"

// ApidPrefixToLabelToken returns the label-key token associated with an apid
// prefix. It strips the trailing underscore so e.g. "cxr_" becomes "cxr" and
// "cxn_" becomes "cxn". Used to build apxy/ keys whose <rt> segment matches
// the resource's id prefix.
func ApidPrefixToLabelToken(p apid.Prefix) string {
	return strings.TrimSuffix(string(p), "_")
}

// BuildImplicitIdentifierLabelsForToken builds the apxy/<rt>/-/id and
// apxy/<rt>/-/ns implicit identifier labels for any resource type, keyed by
// the supplied <rt> token and identifier string. This is the underlying
// builder used by both apid-keyed and path-keyed resources.
func BuildImplicitIdentifierLabelsForToken(rt, id, namespacePath string) Labels {
	if rt == "" || id == "" {
		return nil
	}
	return Labels{
		fmt.Sprintf("%s%s/%s/id", ApxyReservedPrefix, rt, ApxyImplicitSegment): id,
		fmt.Sprintf("%s%s/%s/ns", ApxyReservedPrefix, rt, ApxyImplicitSegment): namespacePath,
	}
}

// BuildNamespaceImplicitIdentifierLabels builds the self-implicit identifier
// labels for a namespace. Both the -/id and -/ns labels carry the namespace's
// own path (a namespace is its own namespace).
func BuildNamespaceImplicitIdentifierLabels(path string) Labels {
	return BuildImplicitIdentifierLabelsForToken(NamespaceLabelToken, path, path)
}

// InjectNamespaceSelfImplicitLabels returns a copy of existing with a
// namespace's own apxy/ns/-/id and apxy/ns/-/ns labels added. Mirrors
// InjectSelfImplicitLabels but for path-keyed namespace resources.
func InjectNamespaceSelfImplicitLabels(path string, existing Labels) Labels {
	implicit := BuildNamespaceImplicitIdentifierLabels(path)
	if len(implicit) == 0 {
		if existing == nil {
			return nil
		}
		return existing.Copy()
	}
	out := make(Labels, len(existing)+len(implicit))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range implicit {
		out[k] = v
	}
	return out
}

// BuildImplicitIdentifierLabels returns the two implicit identifier labels
// for an apid-keyed resource: apxy/<rt>/-/id and apxy/<rt>/-/ns, where <rt>
// is derived from the resource's id prefix. The id and namespacePath are
// stored as label values verbatim — callers must ensure they have already
// been validated by their respective rules.
func BuildImplicitIdentifierLabels(id apid.ID, namespacePath string) Labels {
	if id.IsNil() {
		return nil
	}
	return BuildImplicitIdentifierLabelsForToken(ApidPrefixToLabelToken(id.Prefix()), string(id), namespacePath)
}

// InjectSelfImplicitLabels returns a copy of existing with the resource's own
// apxy/<rt>/-/id and apxy/<rt>/-/ns labels added. The self-implicit labels
// override any same-keyed entries already in existing (deeper-overrides-
// shallower across the carry-forward chain). Callers pass this to the create
// path so the row is persisted with the implicit identifier labels in place.
func InjectSelfImplicitLabels(id apid.ID, namespacePath string, existing Labels) Labels {
	implicit := BuildImplicitIdentifierLabels(id, namespacePath)
	if len(implicit) == 0 {
		if existing == nil {
			return nil
		}
		return existing.Copy()
	}
	out := make(Labels, len(existing)+len(implicit))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range implicit {
		out[k] = v
	}
	return out
}

// SplitUserAndApxyLabels partitions a labels map into the user-provided
// portion (no apxy/ prefix) and the system-managed portion (apxy/ prefix).
// The two returned maps are disjoint and together reconstitute the input.
// Either map may be nil if its half is empty.
func SplitUserAndApxyLabels(labels Labels) (user, apxy Labels) {
	for k, v := range labels {
		if strings.HasPrefix(k, ApxyReservedPrefix) {
			if apxy == nil {
				apxy = make(Labels)
			}
			apxy[k] = v
		} else {
			if user == nil {
				user = make(Labels)
			}
			user[k] = v
		}
	}
	return user, apxy
}

// MergeApxyAndUserLabels returns a single map containing both the user and
// apxy portions. Because the two inputs are partitioned by key prefix, no
// collisions are possible.
func MergeApxyAndUserLabels(user, apxy Labels) Labels {
	if len(user) == 0 && len(apxy) == 0 {
		return nil
	}
	out := make(Labels, len(user)+len(apxy))
	for k, v := range user {
		out[k] = v
	}
	for k, v := range apxy {
		out[k] = v
	}
	return out
}

// ParentCarryForward bundles a parent's resource-type token with the
// parent's stored labels for use with ApplyParentCarryForward.
type ParentCarryForward struct {
	Rt     string
	Labels Labels
}

// ApplyParentCarryForward composes a child resource's labels from the
// parents listed and the user-supplied labels. For each parent it calls
// BuildCarriedLabels(parent.Rt, parent.Labels) and merges the result; the
// user's own labels are merged last among non-self entries (they cannot
// collide with apxy/ keys because user input cannot reference the apxy/
// namespace). Parents are applied in order, so a later parent's apxy/
// pass-through overrides an earlier parent's — list parents from most
// distant to most direct so the most direct ancestor wins on conflicts
// (deeper-overrides-shallower).
//
// Callers should follow with InjectSelfImplicitLabels (or
// InjectNamespaceSelfImplicitLabels for path-keyed namespaces) so the
// child's own self-implicit labels override any same-keyed pass-through
// from a parent.
func ApplyParentCarryForward(userLabels Labels, parents ...ParentCarryForward) Labels {
	out := make(Labels)
	for _, p := range parents {
		for k, v := range BuildCarriedLabels(p.Rt, p.Labels) {
			out[k] = v
		}
	}
	for k, v := range userLabels {
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// fetchLabelsForCarryForward returns the labels column for a row identified
// by `where` in `table`, or nil if the row does not exist. Parent rows are
// expected for carry-forward materialization but are not strictly required —
// a missing parent simply yields no carry-forward and the daily consistency
// checker can reconcile later if the parent appears.
func (s *service) fetchLabelsForCarryForward(ctx context.Context, runner sq.BaseRunner, table string, where sq.Eq) (Labels, error) {
	var labels Labels
	err := s.sq.
		Select("labels").
		From(table).
		Where(where).
		RunWith(runner).
		QueryRow().
		Scan(&labels)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return labels, nil
}
