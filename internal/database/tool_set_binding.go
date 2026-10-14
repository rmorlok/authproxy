package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolSetBindingsTable stores the lifetime of a ToolSet-to-connection assignment.
const ToolSetBindingsTable = "tool_set_bindings"

var (
	// ErrToolSetBindingEpochConflict means a worker's captured binding is stale.
	ErrToolSetBindingEpochConflict = errors.New("toolset binding epoch changed")

	// ErrToolSetBindingMembershipConflict means current namespace/label matching
	// contradicts the requested creation or removal. Recompute desired membership.
	ErrToolSetBindingMembershipConflict = errors.New("toolset binding membership changed")
)

// ToolSetBinding is an internal assignment identity, initially pending inventory
// installation. It is not a public resource or an execution grant. Removing and
// recreating the same pair allocates a new ID; a removed ID is never revived.
// Applied generations and inventory pointers must be added together with atomic
// inventory operations, not independently changed by these membership methods.
type ToolSetBinding struct {
	Id           apid.ID
	ToolSetId    apid.ID
	ConnectionId apid.ID

	// Epoch starts at one and advances on removal. Future inventory commits must
	// advance it too, alongside selector and connection-context preconditions.
	// It protects one assignment's lifetime, not the lifetime of the parent pair.
	Epoch     uint64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// cols lists binding columns in migration order, matching fields and values.
func (b *ToolSetBinding) cols() []string {
	return []string{
		"id",
		"tool_set_id",
		"connection_id",
		"epoch",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

// fields supplies destinations for scanning one internal assignment.
func (b *ToolSetBinding) fields() []any {
	return []any{
		&b.Id,
		&b.ToolSetId,
		&b.ConnectionId,
		&b.Epoch,
		&b.CreatedAt,
		&b.UpdatedAt,
		&b.DeletedAt,
	}
}

// values supplies the complete assignment row for insertion.
func (b *ToolSetBinding) values() []any {
	return []any{
		b.Id,
		b.ToolSetId,
		b.ConnectionId,
		b.Epoch,
		b.CreatedAt,
		b.UpdatedAt,
		b.DeletedAt,
	}
}

// Validate checks typed identities and a positive epoch in the shared SQL range.
// Current parent liveness and membership require transactional database checks.
func (b *ToolSetBinding) Validate() error {
	if b == nil {
		return errors.New("toolset binding is required")
	}

	if err := validateToolSetBindingID(b.Id); err != nil {
		return err
	}

	if err := validateToolSetBindingParents(b.ToolSetId, b.ConnectionId); err != nil {
		return err
	}

	if b.Epoch == 0 || b.Epoch > math.MaxInt64 {
		return errors.New("toolset binding epoch must be a positive signed 64-bit integer")
	}

	return nil
}

// validateToolSetBindingID rejects absent IDs and IDs of public parent resources.
func validateToolSetBindingID(id apid.ID) error {
	if id.IsNil() {
		return errors.New("toolset binding id is required")
	}

	return id.ValidatePrefix(apid.PrefixToolSetBinding)
}

// validateToolSetBindingParents checks the immutable typed parent identities.
func validateToolSetBindingParents(toolSetId, connectionId apid.ID) error {
	if err := validateToolSetID(toolSetId); err != nil {
		return err
	}

	if connectionId.IsNil() {
		return errors.New("connection id is required")
	}

	return connectionId.ValidatePrefix(apid.PrefixConnection)
}

// validateBindingSelectorRevision rejects missing or unrepresentable snapshots.
func validateBindingSelectorRevision(revision uint64) error {
	if revision == 0 || revision > math.MaxInt64 {
		return errors.New("expected selector revision must be a positive signed 64-bit integer")
	}

	return nil
}

// lockToolSetBindingMembership locks both parents, in ToolSet then Connection
// order, before any transaction reads. The neutral writes preserve metadata and
// avoid SQLite read-to-write upgrades. Current persisted labels and selector
// policy remain stable until commit, even if a label update is queued elsewhere.
// Deleted parents are included so cleanup remains possible; in that case no
// selector is compiled and parentsLive is false. No credential/config data is read.
func (s *service) lockToolSetBindingMembership(
	ctx context.Context,
	tx *sql.Tx,
	toolSetId apid.ID,
	connectionId apid.ID,
) (selectorRevision uint64, selected bool, parentsLive bool, err error) {
	for _, parent := range []struct {
		table string
		id    apid.ID
	}{
		{ToolSetsTable, toolSetId},
		{ConnectionsTable, connectionId},
	} {
		result, err := s.sq.
			Update(parent.table).
			Set("id", parent.id).
			Where(sq.Eq{"id": parent.id}).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return 0, false, false, err
		}

		count, err := result.RowsAffected()
		if err != nil {
			return 0, false, false, err
		}
		if count == 0 {
			return 0, false, false, ErrNotFound
		}
	}

	var toolSet ToolSet
	err = s.sq.
		Select(
			"namespace",
			"connection_selector",
			"selector_revision",
			"deleted_at",
		).
		From(ToolSetsTable).
		Where(sq.Eq{"id": toolSetId}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(
			&toolSet.Namespace,
			(*toolSetSelectorDB)(&toolSet.ConnectionSelector),
			&toolSet.SelectorRevision,
			&toolSet.DeletedAt,
		)
	if err != nil {
		return 0, false, false, err
	}

	var connection Connection
	err = s.sq.
		Select(
			"namespace",
			"labels",
			"deleted_at",
		).
		From(ConnectionsTable).
		Where(sq.Eq{"id": connectionId}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(
			&connection.Namespace,
			&connection.Labels,
			&connection.DeletedAt,
		)
	if err != nil {
		return 0, false, false, err
	}

	if toolSet.DeletedAt != nil || connection.DeletedAt != nil {
		return toolSet.SelectorRevision, false, false, nil
	}

	namespaceMatcher, labelSelector, err := toolSet.ConnectionSelector.Compile(toolSet.Namespace)
	if err != nil {
		return 0, false, false, err
	}

	labels, err := ParseLabelSelector(labelSelector)
	if err != nil {
		return 0, false, false, err
	}

	selected = namespaceschema.Matches(namespaceMatcher, connection.Namespace) &&
		labels.Matches(connection.Labels)

	return toolSet.SelectorRevision, selected, true, nil
}

// CreateToolSetBinding creates a fresh pending identity only when both parents
// are live and current labels match the expected selector policy. Setup, health,
// and primary-generation readiness do not prevent recording pending membership.
// An existing live pair returns ErrDuplicate; this never revives a tombstone.
func (s *service) CreateToolSetBinding(
	ctx context.Context,
	toolSetId apid.ID,
	connectionId apid.ID,
	expectedSelectorRevision uint64,
) (*ToolSetBinding, error) {
	if err := validateToolSetBindingParents(toolSetId, connectionId); err != nil {
		return nil, err
	}
	if err := validateBindingSelectorRevision(expectedSelectorRevision); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	revision, selected, live, err := s.lockToolSetBindingMembership(ctx, tx, toolSetId, connectionId)
	if err != nil {
		return nil, err
	}
	if !live {
		return nil, ErrNotFound
	}
	if revision != expectedSelectorRevision {
		return nil, ErrToolSetSelectorRevisionConflict
	}
	if !selected {
		return nil, ErrToolSetBindingMembershipConflict
	}

	now := apctx.GetClock(ctx).Now()
	result := ToolSetBinding{
		Id:           apid.New(apid.PrefixToolSetBinding),
		ToolSetId:    toolSetId,
		ConnectionId: connectionId,
		Epoch:        1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err = s.sq.
		Insert(ToolSetBindingsTable).
		Columns(result.cols()...).
		Values(result.values()...).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, wrapDatabaseMutationError("failed to create toolset binding", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDatabaseMutationError("failed to commit toolset binding", err)
	}
	return &result, nil
}

// getToolSetBinding reads an undeleted assignment through the supplied runner.
// Public getters require live parents; cleanup uses the raw identity even after
// parent deletion. Conditions use the b alias and must select a unique row.
func (s *service) getToolSetBinding(
	ctx context.Context,
	runner sq.BaseRunner,
	where sq.Eq,
	requireLiveParents bool,
) (*ToolSetBinding, error) {
	var result ToolSetBinding

	columns := util.PrependStringToAll("b.", result.cols())
	query := s.sq.
		Select(columns...).
		From(ToolSetBindingsTable + " b").
		Where(where).
		Where(sq.Eq{"b.deleted_at": nil})
	if requireLiveParents {
		query = query.
			Join(ToolSetsTable + " t ON t.id = b.tool_set_id").
			Join(ConnectionsTable + " c ON c.id = b.connection_id").
			Where(sq.Eq{"t.deleted_at": nil, "c.deleted_at": nil})
	}
	err := query.
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

// GetToolSetBinding finds an assignment by its internal lifetime identity.
// A selector change alone does not hide it; reconciliation performs removal.
func (s *service) GetToolSetBinding(
	ctx context.Context,
	id apid.ID,
) (*ToolSetBinding, error) {
	if err := validateToolSetBindingID(id); err != nil {
		return nil, err
	}
	return s.getToolSetBinding(ctx, s.db, sq.Eq{"b.id": id}, true)
}

// GetToolSetBindingForConnection returns the live assignment for one parent pair.
func (s *service) GetToolSetBindingForConnection(
	ctx context.Context,
	toolSetId apid.ID,
	connectionId apid.ID,
) (*ToolSetBinding, error) {
	if err := validateToolSetBindingParents(toolSetId, connectionId); err != nil {
		return nil, err
	}
	return s.getToolSetBinding(ctx, s.db, sq.Eq{
		"b.tool_set_id":   toolSetId,
		"b.connection_id": connectionId,
	}, true)
}

// DeleteToolSetBinding retires the captured assignment only if membership no
// longer matches the expected current selector, or a parent has been deleted.
// Deleted-parent cleanup ignores expectedSelectorRevision (including zero).
// Epoch is always checked and incremented atomically; an old identity can never
// remove a later assignment for the same pair. This handles pending membership,
// not an installed inventory or execution-readiness decision.
func (s *service) DeleteToolSetBinding(
	ctx context.Context,
	id apid.ID,
	expectedEpoch uint64,
	expectedSelectorRevision uint64,
) error {
	if err := validateToolSetBindingID(id); err != nil {
		return err
	}
	if expectedEpoch == 0 || expectedEpoch >= math.MaxInt64 {
		return errors.New("expected binding epoch must be positive and allow a signed 64-bit increment")
	}
	// Read immutable parent IDs outside the transaction, then lock them before
	// its first read. The binding is re-read and fenced after those locks.
	identity, err := s.getToolSetBinding(ctx, s.db, sq.Eq{"b.id": id}, false)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	revision, selected, live, err := s.lockToolSetBindingMembership(ctx, tx, identity.ToolSetId, identity.ConnectionId)
	if err != nil {
		return err
	}

	current, err := s.getToolSetBinding(ctx, tx, sq.Eq{"b.id": id}, false)
	if err != nil {
		return err
	}

	if current.Epoch != expectedEpoch {
		return ErrToolSetBindingEpochConflict
	}
	if live {
		if err := validateBindingSelectorRevision(expectedSelectorRevision); err != nil {
			return err
		}
		if revision != expectedSelectorRevision {
			return ErrToolSetSelectorRevisionConflict
		}
		if selected {
			return ErrToolSetBindingMembershipConflict
		}
	}

	now := apctx.GetClock(ctx).Now()
	result, err := s.sq.
		Update(ToolSetBindingsTable).
		Set("epoch", sq.Expr("epoch + 1")).
		Set("deleted_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"id": id, "epoch": expectedEpoch, "deleted_at": nil}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove toolset binding: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrToolSetBindingEpochConflict
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit toolset binding removal: %w", err)
	}

	return nil
}
