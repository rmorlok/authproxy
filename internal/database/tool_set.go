package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// ToolSetsTable stores logical ToolSet identity and connection membership policy.
const ToolSetsTable = "tool_sets"

// ErrToolSetSelectorRevisionConflict means a selector replacement lost its
// compare-and-increment check. The caller must read current state before retrying.
var ErrToolSetSelectorRevisionConflict = errors.New("toolset selector revision changed")

// ToolSet is the logical, namespace-owned record shared by all generations.
// Definitions and publication state belong to separate generation records;
// neither publishing nor migrating a generation rewrites this selection policy.
type ToolSet struct {
	Id                 apid.ID
	Namespace          string
	Name               common.ResourceName
	Labels             Labels
	Annotations        Annotations
	ConnectionSelector toolsetschema.ConnectionSelector

	// SelectorRevision starts at one and advances with each accepted selector
	// replacement. It fences membership work independently of generation numbers.
	SelectorRevision uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// cols lists columns in the same order as the migrations, fields, and values.
func (t *ToolSet) cols() []string {
	return []string{
		"id",
		"namespace",
		"name",
		"labels",
		"annotations",
		"connection_selector",
		"selector_revision",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

// fields supplies scan destinations, adapting only the selector's SQL encoding.
func (t *ToolSet) fields() []any {
	return []any{
		&t.Id,
		&t.Namespace,
		&t.Name,
		&t.Labels,
		&t.Annotations,
		(*toolSetSelectorDB)(&t.ConnectionSelector),
		&t.SelectorRevision,
		&t.CreatedAt,
		&t.UpdatedAt,
		&t.DeletedAt,
	}
}

// values supplies insertion values without flattening the canonical selector.
func (t *ToolSet) values() []any {
	return []any{
		t.Id,
		t.Namespace,
		t.Name,
		t.Labels,
		t.Annotations,
		toolSetSelectorDB(t.ConnectionSelector),
		t.SelectorRevision,
		t.CreatedAt,
		t.UpdatedAt,
		t.DeletedAt,
	}
}

// Validate checks the stored identity, metadata, selection scope, and positive
// revision. SQL counters use the signed 64-bit range shared by both providers.
func (t *ToolSet) Validate() error {
	if t == nil {
		return errors.New("toolset is required")
	}

	if err := validateToolSetID(t.Id); err != nil {
		return err
	}

	if err := namespace.ValidatePath(t.Namespace); err != nil {
		return fmt.Errorf("invalid toolset namespace: %w", err)
	}

	if err := t.Name.Validate(); err != nil {
		return fmt.Errorf("invalid toolset name: %w", err)
	}

	if err := t.Labels.Validate(); err != nil {
		return fmt.Errorf("invalid toolset labels: %w", err)
	}

	if err := t.Annotations.Validate(); err != nil {
		return fmt.Errorf("invalid toolset annotations: %w", err)
	}

	if err := t.ConnectionSelector.ValidateForNamespace(t.Namespace, nil); err != nil {
		return fmt.Errorf("invalid toolset connection selector: %w", err)
	}

	if t.SelectorRevision == 0 || t.SelectorRevision > math.MaxInt64 {
		return errors.New("toolset selector revision must be a positive signed 64-bit integer")
	}

	return nil
}

// toolSetSelectorDB adds SQL scanning to the canonical selector. Unlike Labels'
// SQL encoding, it preserves matchLabels: {} instead of turning it into NULL.
type toolSetSelectorDB toolsetschema.ConnectionSelector

// Value stores the canonical selector JSON in either a text or jsonb column.
func (d toolSetSelectorDB) Value() (driver.Value, error) {
	encoded, err := json.Marshal(toolsetschema.ConnectionSelector(d))
	return string(encoded), err
}

// Scan decodes either provider's JSON representation using the schema's decoder.
// A NULL selector is invalid; reads must not silently broaden a missing policy.
func (d *toolSetSelectorDB) Scan(src any) error {
	var data []byte
	switch value := src.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		return fmt.Errorf("toolSetSelectorDB: cannot scan %T", src)
	}

	return json.Unmarshal(data, (*toolsetschema.ConnectionSelector)(d))
}

// validateToolSetID rejects missing IDs and IDs of other resource types.
func validateToolSetID(id apid.ID) error {
	if id.IsNil() {
		return errors.New("toolset id is required")
	}
	if err := id.ValidatePrefix(apid.PrefixToolSet); err != nil {
		return fmt.Errorf("invalid toolset id: %w", err)
	}

	return nil
}

// CreateToolSet inserts a live logical record in an existing live namespace.
// It initializes revision/timestamps, defaults the name to the ID, and derives
// inherited/self labels. The caller's record changes only after commit succeeds.
func (s *service) CreateToolSet(ctx context.Context, toolSet *ToolSet) error {
	if toolSet == nil {
		return errors.New("toolset is required")
	}

	candidate := *toolSet
	candidate.ConnectionSelector = *toolSet.ConnectionSelector.Clone()
	candidate.Labels = toolSet.Labels.Copy()

	if candidate.Name == "" {
		candidate.Name = common.ResourceName(candidate.Id.String())
	}

	candidate.SelectorRevision = 1
	candidate.DeletedAt = nil
	candidate.CreatedAt = toolStorageTime(ctx)
	candidate.UpdatedAt = candidate.CreatedAt

	if err := candidate.Validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var parentLabels Labels
	err = s.sq.
		Select("labels").
		From(NamespacesTable).
		Where(sq.Eq{
			"path":       candidate.Namespace,
			"deleted_at": nil,
		}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&parentLabels)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNamespaceDoesNotExist
	}
	if err != nil {
		return err
	}

	candidate.Labels = ApplyParentCarryForward(
		candidate.Labels,
		ParentCarryForward{
			Rt:     NamespaceLabelToken,
			Labels: parentLabels,
		},
	)

	candidate.Labels = InjectSelfImplicitLabels(
		candidate.Id,
		candidate.Name,
		candidate.Namespace,
		candidate.Labels,
	)
	_, err = s.sq.
		Insert(ToolSetsTable).
		Columns(candidate.cols()...).
		Values(candidate.values()...).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return wrapDatabaseMutationError("failed to create toolset", err)
	}

	if err := tx.Commit(); err != nil {
		return wrapDatabaseMutationError("failed to commit toolset", err)
	}

	*toolSet = candidate

	return nil
}

// getToolSet reads one live logical record through the supplied database runner.
// Callers supply a unique identity; missing and soft-deleted records are hidden.
func (s *service) getToolSet(
	ctx context.Context,
	runner sq.BaseRunner,
	where sq.Eq,
) (*ToolSet, error) {
	var result ToolSet
	err := s.sq.
		Select(result.cols()...).
		From(ToolSetsTable).
		Where(where).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(runner).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetToolSet returns the live logical ToolSet with the specified immutable ID.
func (s *service) GetToolSet(ctx context.Context, id apid.ID) (*ToolSet, error) {
	if err := validateToolSetID(id); err != nil {
		return nil, err
	}
	return s.getToolSet(ctx, s.db, sq.Eq{"id": id})
}

// GetToolSetByName returns a live ToolSet in one exact namespace; it does not
// perform ancestor searches or select any definition generation.
func (s *service) GetToolSetByName(
	ctx context.Context,
	ns string,
	name common.ResourceName,
) (*ToolSet, error) {
	if err := namespace.ValidatePath(ns); err != nil {
		return nil, err
	}
	if err := name.Validate(); err != nil {
		return nil, err
	}

	return s.getToolSet(ctx, s.db, sq.Eq{"namespace": ns, "name": name})
}

// UpdateToolSetConnectionSelector atomically replaces a live ToolSet's policy
// only if expectedRevision is still current. Every accepted write advances the
// revision, even for identical policy. Membership jobs use it to detect stale
// work; no generation or installed inventory is changed here.
func (s *service) UpdateToolSetConnectionSelector(
	ctx context.Context,
	id apid.ID,
	expectedRevision uint64,
	selector toolsetschema.ConnectionSelector,
) (*ToolSet, error) {
	if expectedRevision == 0 || expectedRevision >= math.MaxInt64 {
		return nil, errors.New("expected selector revision must be positive and allow a signed 64-bit increment")
	}

	existing, err := s.GetToolSet(ctx, id)
	if err != nil {
		return nil, err
	}

	if existing.SelectorRevision != expectedRevision {
		return nil, ErrToolSetSelectorRevisionConflict
	}

	if err := selector.ValidateForNamespace(
		existing.Namespace,
		nil, // validation context
	); err != nil {
		return nil, fmt.Errorf("invalid toolset connection selector: %w", err)
	}

	// The read above supplies immutable ownership for validation, not a lock.
	// The conditional UPDATE provides the concurrency guarantee in both stores.
	var result ToolSet
	err = s.sq.
		Update(ToolSetsTable).
		Set("connection_selector", toolSetSelectorDB(selector)).
		Set("selector_revision", sq.Expr("selector_revision + 1")).
		Set("updated_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{
			"id":                id,
			"selector_revision": expectedRevision,
			"deleted_at":        nil,
		}).
		Suffix("RETURNING " + strings.Join(result.cols(), ", ")).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		if _, readErr := s.GetToolSet(ctx, id); readErr != nil {
			return nil, readErr
		}
		return nil, ErrToolSetSelectorRevisionConflict
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateToolSetMetadata replaces a live ToolSet's name, user labels, and
// annotations in one transaction. userLabels must not contain apxy/ keys; the
// existing system labels are retained and the self-implicit identity labels are
// re-derived for the new name. It never touches the selector, its revision, or
// any generation. A name already used in the namespace returns ErrDuplicate.
func (s *service) UpdateToolSetMetadata(
	ctx context.Context,
	id apid.ID,
	name common.ResourceName,
	userLabels map[string]string,
	annotations map[string]string,
) (*ToolSet, error) {
	if err := validateToolSetID(id); err != nil {
		return nil, err
	}
	if err := name.Validate(); err != nil {
		return nil, fmt.Errorf("invalid toolset name: %w", err)
	}
	if err := smeta.ValidateUserLabels(userLabels); err != nil {
		return nil, fmt.Errorf("invalid toolset labels: %w", err)
	}
	if err := smeta.ValidateAnnotations(annotations); err != nil {
		return nil, fmt.Errorf("invalid toolset annotations: %w", err)
	}

	var result *ToolSet
	err := s.transaction(func(tx *sql.Tx) error {
		// PostgreSQL locks the row so a concurrent system-label refresh between
		// this read and the write cannot be lost; SQLite serializes writers.
		query := s.sq.
			Select(new(ToolSet).cols()...).
			From(ToolSetsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil})
		if s.cfg.GetProvider() == sconfig.DatabaseProviderPostgres {
			query = query.Suffix("FOR UPDATE")
		}

		var current ToolSet
		if err := query.RunWith(tx).QueryRowContext(ctx).Scan(current.fields()...); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		_, apxy := SplitUserAndApxyLabels(current.Labels)
		labels := InjectSelfImplicitLabels(
			id,
			name,
			current.Namespace,
			MergeApxyAndUserLabels(Labels(userLabels), apxy),
		)

		updated := current
		updated.Name = name
		updated.Labels = labels
		updated.Annotations = Annotations(annotations)
		updated.UpdatedAt = toolStorageTime(ctx)

		_, err := s.sq.
			Update(ToolSetsTable).
			Set("name", updated.Name).
			Set("labels", updated.Labels).
			Set("annotations", updated.Annotations).
			Set("updated_at", updated.UpdatedAt).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return wrapDatabaseMutationError("failed to update toolset metadata", err)
		}

		result = &updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteToolSet soft-deletes one live logical record, releasing its name while
// retaining its immutable ID. Generation/binding cleanup is added with those
// stores; their execution paths must check logical parent liveness.
func (s *service) DeleteToolSet(ctx context.Context, id apid.ID) error {
	if err := validateToolSetID(id); err != nil {
		return err
	}

	now := apctx.GetClock(ctx).Now()
	result, err := s.sq.
		Update(ToolSetsTable).
		Set("deleted_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}
