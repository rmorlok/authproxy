package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolsTable holds Tool identities and their current definition pointers.
const ToolsTable = "tools"

// ErrToolRevisionConflict means a replacement or deletion used a stale revision.
// Callers must reload and validate their candidate before retrying.
var ErrToolRevisionConflict = errors.New("tool revision changed")

// Tool is the identity of a standalone operation bound to one connection in its
// namespace. A replacement changes its active revision atomically, while prior
// definitions remain immutable and inspectable. This initial storage model does
// not yet include metadata/search projections or generated Tool ownership; those
// must be integrated before exposing management or managed-inventory operations.
type Tool struct {
	Id             apid.ID
	Namespace      string
	Name           common.ResourceName
	ConnectionId   apid.ID
	ActiveRevision uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

// cols lists identity columns in migration order, matching fields and values.
func (t *Tool) cols() []string {
	return []string{
		"id",
		"namespace",
		"name",
		"connection_id",
		"active_revision",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

// fields supplies destinations for reading one Tool identity.
func (t *Tool) fields() []any {
	return []any{
		&t.Id,
		&t.Namespace,
		&t.Name,
		&t.ConnectionId,
		&t.ActiveRevision,
		&t.CreatedAt,
		&t.UpdatedAt,
		&t.DeletedAt,
	}
}

// values supplies the identity row for insertion.
func (t *Tool) values() []any {
	return []any{
		t.Id,
		t.Namespace,
		t.Name,
		t.ConnectionId,
		t.ActiveRevision,
		t.CreatedAt,
		t.UpdatedAt,
		t.DeletedAt,
	}
}

// Validate checks identity shape and the representable active revision. Parent
// liveness and connection namespace equality require transactional checks.
func (t *Tool) Validate() error {
	if t == nil {
		return errors.New("tool is required")
	}

	if err := validateToolID(t.Id); err != nil {
		return err
	}

	if err := namespace.ValidatePath(t.Namespace); err != nil {
		return fmt.Errorf("invalid tool namespace: %w", err)
	}

	if err := t.Name.Validate(); err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	if err := validateToolConnectionID(t.ConnectionId); err != nil {
		return err
	}

	return validateToolRevision(t.ActiveRevision)
}

// validateToolID rejects absent IDs and IDs of other resource kinds.
func validateToolID(id apid.ID) error {
	if id.IsNil() {
		return errors.New("tool id is required")
	}
	return id.ValidatePrefix(apid.PrefixTool)
}

// validateToolConnectionID checks a resolved connection identity.
func validateToolConnectionID(id apid.ID) error {
	if id.IsNil() {
		return errors.New("tool connection id is required")
	}
	return id.ValidatePrefix(apid.PrefixConnection)
}

// lockToolConnections serializes writes with connection deletion and purge.
// Lock all captured parents in ID order before the Tool, matching the cascade
// direction used by purge. Neutral writes also avoid SQLite snapshot upgrades.
// Deleted connections are locked too so a Tool can be retargeted for recovery.
func (s *service) lockToolConnections(
	ctx context.Context,
	tx *sql.Tx,
	ids ...apid.ID,
) error {
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		result, err := s.sq.Update(ConnectionsTable).
			Set("id", id).
			Where(sq.Eq{"id": id}).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return err
		}

		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
	}

	return nil
}

// checkToolConnection verifies the locked destination and its namespace are live
// and that the Tool stays within that exact namespace. It reads no credentials
// or connection configuration. Setup/health does not prevent storing definitions.
func (s *service) checkToolConnection(
	ctx context.Context,
	tx *sql.Tx,
	tool *Tool,
) error {
	var ns string
	err := s.sq.Select("c.namespace").
		From(ConnectionsTable + " c").
		Join(NamespacesTable + " n ON n.path = c.namespace").
		Where(sq.Eq{
			"c.id":         tool.ConnectionId,
			"c.deleted_at": nil,
			"n.deleted_at": nil,
		}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&ns)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if ns != tool.Namespace {
		return errors.New("tool and connection must have the same namespace")
	}

	return nil
}

// CreateStandaloneTool stores a new identity and its active revision together.
// The caller supplies a validated, compiled, encrypted definition and its digest;
// this layer does not decrypt or compile it. Names default to the allocated ID.
// Only a successful commit changes the caller's record. Existing IDs, including
// tombstones, are never revived; duplicate live namespace/name pairs conflict.
func (s *service) CreateStandaloneTool(
	ctx context.Context,
	tool *Tool,
	definition encfield.EncryptedField,
	digest string,
) (*ToolRevision, error) {
	if tool == nil {
		return nil, errors.New("tool is required")
	}

	candidate := *tool
	if candidate.Name == "" {
		candidate.Name = common.ResourceName(candidate.Id.String())
	}
	candidate.ActiveRevision = 1
	candidate.CreatedAt = apctx.GetClock(ctx).Now()
	candidate.UpdatedAt = candidate.CreatedAt
	candidate.DeletedAt = nil

	if err := candidate.Validate(); err != nil {
		return nil, err
	}

	revision := newToolRevision(ctx, &candidate, definition, digest)
	if err := revision.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.lockToolConnections(ctx, tx, candidate.ConnectionId); err != nil {
		return nil, err
	}
	if err := s.checkToolConnection(ctx, tx, &candidate); err != nil {
		return nil, err
	}

	_, err = s.sq.Insert(ToolsTable).
		Columns(candidate.cols()...).
		Values(candidate.values()...).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, wrapDatabaseMutationError("failed to create tool", err)
	}
	if err := s.insertToolRevision(ctx, tx, revision); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDatabaseMutationError("failed to commit tool creation", err)
	}

	*tool = candidate

	return revision, nil
}

// getTool reads a live identity in a live namespace. Connection liveness is
// intentionally not required for management inspection, retargeting, or cleanup.
func (s *service) getTool(
	ctx context.Context,
	runner sq.BaseRunner,
	id apid.ID,
) (*Tool, error) {
	var result Tool
	columns := util.PrependStringToAll("t.", result.cols())

	err := s.sq.Select(columns...).
		From(ToolsTable + " t").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Where(sq.Eq{"t.id": id, "t.deleted_at": nil, "n.deleted_at": nil}).
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

// GetTool returns identity and the active pointer for management. Reading this
// then separately reading a revision is not an admission snapshot; use the joined
// GetCurrentToolRevision for a consistent current definition and connection.
func (s *service) GetTool(ctx context.Context, id apid.ID) (*Tool, error) {
	if err := validateToolID(id); err != nil {
		return nil, err
	}
	return s.getTool(ctx, s.db, id)
}

// UpdateStandaloneTool appends an immutable revision and atomically replaces the
// name, connection, and active pointer. The namespace cannot change. Every
// accepted update advances the revision, even for identical content. Definitions
// must already be validated and compiled by the caller. Failed or stale writes
// preserve history and leave the caller's record unchanged.
func (s *service) UpdateStandaloneTool(
	ctx context.Context,
	tool *Tool,
	expectedRevision uint64,
	definition encfield.EncryptedField,
	digest string,
) (*ToolRevision, error) {
	if tool == nil {
		return nil, errors.New("tool is required")
	}
	if err := validateToolRevision(expectedRevision); err != nil {
		return nil, err
	}
	if expectedRevision == math.MaxInt64 {
		return nil, ErrToolRevisionConflict
	}
	candidate := *tool
	candidate.ActiveRevision = expectedRevision + 1
	if err := candidate.Validate(); err != nil {
		return nil, err
	}
	revision := newToolRevision(ctx, &candidate, definition, digest)
	if err := revision.Validate(); err != nil {
		return nil, err
	}
	// Capture only the lock target outside the transaction. It is rechecked
	// after locking; a concurrent retarget cannot leave a parent unprotected.
	observed, err := s.GetTool(ctx, candidate.Id)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.lockToolConnections(ctx, tx, observed.ConnectionId, candidate.ConnectionId); err != nil {
		return nil, err
	}
	_, err = s.sq.Update(ToolsTable).
		Set("id", candidate.Id).
		Where(sq.Eq{"id": candidate.Id, "deleted_at": nil}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, err
	}
	current, err := s.getTool(ctx, tx, candidate.Id)
	if err != nil {
		return nil, err
	}
	if current.ActiveRevision != expectedRevision || current.ConnectionId != observed.ConnectionId {
		return nil, ErrToolRevisionConflict
	}
	if current.Namespace != candidate.Namespace {
		return nil, errors.New("tool namespace cannot change")
	}
	if err := s.checkToolConnection(ctx, tx, &candidate); err != nil {
		return nil, err
	}
	if err := s.insertToolRevision(ctx, tx, revision); err != nil {
		return nil, err
	}
	candidate.CreatedAt = current.CreatedAt
	candidate.UpdatedAt = revision.CreatedAt
	candidate.DeletedAt = nil
	result, err := s.sq.Update(ToolsTable).
		Set("name", candidate.Name).
		Set("connection_id", candidate.ConnectionId).
		Set("active_revision", candidate.ActiveRevision).
		Set("updated_at", candidate.UpdatedAt).
		Where(sq.Eq{"id": candidate.Id, "active_revision": expectedRevision, "deleted_at": nil}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, wrapDatabaseMutationError("failed to replace tool", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrToolRevisionConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDatabaseMutationError("failed to commit tool replacement", err)
	}
	*tool = candidate
	return revision, nil
}

// DeleteStandaloneTool soft-deletes exactly the observed live revision, releasing
// its name while retaining the identity and revision history. Deleted connections
// do not block cleanup. A removed ID is never reused by the create operation.
func (s *service) DeleteStandaloneTool(ctx context.Context, id apid.ID, expectedRevision uint64) error {
	if err := validateToolID(id); err != nil {
		return err
	}
	if err := validateToolRevision(expectedRevision); err != nil {
		return err
	}
	now := apctx.GetClock(ctx).Now()
	result, err := s.sq.Update(ToolsTable).
		Set("deleted_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"id": id, "active_revision": expectedRevision, "deleted_at": nil}).
		RunWith(s.db).
		ExecContext(ctx)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.GetTool(ctx, id); err != nil {
			return err
		}
		return ErrToolRevisionConflict
	}
	return nil
}
