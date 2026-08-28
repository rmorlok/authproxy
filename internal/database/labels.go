package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
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

// Validate validates all labels (system mode — apxy/ keys allowed, with the
// longer SystemLabelValueMaxLength value cap for those keys).
func (l Labels) Validate() error {
	if l == nil {
		return nil
	}

	return smeta.ValidateLabels(map[string]string(l))
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

// BuildImplicitResourceLabelsForToken builds the apxy/<rt>/-/id,
// apxy/<rt>/-/name, and apxy/<rt>/-/ns implicit identity labels for any
// resource type, keyed by the supplied <rt> token and identifier string. This
// is the underlying builder used by both apid-keyed and path-keyed resources.
func BuildImplicitResourceLabelsForToken(rt, id string, name scommon.ResourceName, namespacePath string) Labels {
	if rt == "" || id == "" {
		return nil
	}
	if name == "" {
		name = scommon.ResourceName(id)
	}
	return Labels{
		fmt.Sprintf("%s%s/%s/id", smeta.SystemLabelPrefix, rt, smeta.SystemLabelSentinel):   id,
		fmt.Sprintf("%s%s/%s/name", smeta.SystemLabelPrefix, rt, smeta.SystemLabelSentinel): string(name),
		fmt.Sprintf("%s%s/%s/ns", smeta.SystemLabelPrefix, rt, smeta.SystemLabelSentinel):   namespacePath,
	}
}

// BuildNamespaceImplicitResourceLabels builds a namespace's self-implicit
// identity labels. Both -/id and -/ns carry the namespace path, while -/name
// carries the final path segment.
func BuildNamespaceImplicitResourceLabels(path string) Labels {
	return BuildImplicitResourceLabelsForToken(NamespaceLabelToken, path, namespace.NameFromPath(path), path)
}

// InjectNamespaceSelfImplicitLabels returns a copy of existing with a
// namespace's own apxy/ns/-/id, apxy/ns/-/name, and apxy/ns/-/ns labels added. Mirrors
// InjectSelfImplicitLabels but for path-keyed namespace resources.
func InjectNamespaceSelfImplicitLabels(path string, existing Labels) Labels {
	implicit := BuildNamespaceImplicitResourceLabels(path)
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

// BuildImplicitResourceLabels returns the three implicit identity labels for
// an apid-keyed resource: apxy/<rt>/-/id, apxy/<rt>/-/name, and
// apxy/<rt>/-/ns, where <rt> is derived from the resource's id prefix.
func BuildImplicitResourceLabels(id apid.ID, name scommon.ResourceName, namespacePath string) Labels {
	if id.IsNil() {
		return nil
	}
	return BuildImplicitResourceLabelsForToken(ApidPrefixToLabelToken(id.Prefix()), string(id), name, namespacePath)
}

// InjectSelfImplicitLabels returns a copy of existing with the resource's own
// apxy/<rt>/-/id, apxy/<rt>/-/name, and apxy/<rt>/-/ns labels added. The self-implicit labels
// override any same-keyed entries already in existing (deeper-overrides-
// shallower across the carry-forward chain). Callers pass this to the create
// path so the row is persisted with the implicit identity labels in place.
func InjectSelfImplicitLabels(id apid.ID, name scommon.ResourceName, namespacePath string, existing Labels) Labels {
	implicit := BuildImplicitResourceLabels(id, name, namespacePath)
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

// updateResourceNameAndSelfLabels atomically updates a mutable resource name
// and its self-implicit name label. PostgreSQL locks the row between the read
// and write so a concurrent label mutation cannot be lost; SQLite serializes
// the write transaction itself.
func (s *service) updateResourceNameAndSelfLabels(
	ctx context.Context,
	table string,
	id apid.ID,
	name scommon.ResourceName,
) error {
	return s.transaction(func(tx *sql.Tx) error {
		var namespacePath string
		var labels Labels
		query := s.sq.
			Select("namespace", "labels").
			From(table).
			Where(sq.Eq{"id": id, "deleted_at": nil})
		if s.cfg.GetProvider() == sconfig.DatabaseProviderPostgres {
			query = query.Suffix("FOR UPDATE")
		}
		if err := query.RunWith(tx).QueryRowContext(ctx).Scan(&namespacePath, &labels); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("failed to read %s before rename: %w", table, err)
		}

		labels = InjectSelfImplicitLabels(id, name, namespacePath, labels)
		result, err := s.sq.
			Update(table).
			Set("name", name).
			Set("labels", labels).
			Set("updated_at", apctx.GetClock(ctx).Now()).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return wrapDatabaseMutationError(fmt.Sprintf("failed to update %s name", table), err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to update %s name: %w", table, err)
		}
		if affected == 0 {
			return ErrNotFound
		}
		if affected > 1 {
			return fmt.Errorf("multiple %s rows were renamed: %w", table, ErrViolation)
		}
		return nil
	})
}

// SplitUserAndApxyLabels partitions a labels map into the user-provided
// portion (no apxy/ prefix) and the system-managed portion (apxy/ prefix).
// The two returned maps are disjoint and together reconstitute the input.
// Either map may be nil if its half is empty.
func SplitUserAndApxyLabels(labels Labels) (user, apxy Labels) {
	for k, v := range labels {
		if strings.HasPrefix(k, smeta.SystemLabelPrefix) {
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

// MergeUpsertLabels composes the labels to persist on an upsert by
// combining caller-supplied labels with the row's existing apxy/ labels.
//
// User-portion labels come from the caller — fully replacing what was stored
// (write-API endpoints that PATCH user labels go through a different helper).
// apxy/-prefixed labels merge: stored values are preserved by default, and
// any apxy/-prefixed entries the caller passes in override the stored values
// for those specific keys. This lets system code update its own provenance
// markers (e.g. apxy/cxr/source) on an upsert without requiring a separate
// label-mutation call, while still preserving apxy/ labels owned by other
// subsystems (e.g. carry-forward materializations).
func MergeUpsertLabels(callerLabels, existingLabels Labels) Labels {
	newUser, newApxy := SplitUserAndApxyLabels(callerLabels)
	_, existingApxy := SplitUserAndApxyLabels(existingLabels)
	mergedApxy := make(Labels, len(existingApxy)+len(newApxy))
	for k, v := range existingApxy {
		mergedApxy[k] = v
	}
	for k, v := range newApxy {
		mergedApxy[k] = v
	}
	return MergeApxyAndUserLabels(newUser, mergedApxy)
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
		if strings.HasPrefix(k, smeta.SystemLabelPrefix) {
			out[k] = v
			continue
		}
		out[fmt.Sprintf("%s%s/%s", smeta.SystemLabelPrefix, parentRt, k)] = v
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
