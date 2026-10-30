package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolsTable holds Tool identities and their current definition pointers.
const ToolsTable = "tools"

// ErrToolRevisionConflict means a replacement or deletion used a stale revision.
// Callers must reload and validate their candidate before retrying.
var ErrToolRevisionConflict = errors.New("tool revision changed")

// ErrToolManaged means a standalone write attempted to create, change, or remove
// controller-owned state. Generated Tools must be changed through their ToolSet.
var ErrToolManaged = errors.New("tool is managed by a toolset")

// Tool is an operation identity bound to one connection in its namespace. A
// replacement changes its active revision atomically, while prior definitions
// remain immutable and inspectable. Labels include authored values and derived
// parent/self projections. Description and Verbs project the active definition.
// Generated inventories are installed atomically through their binding;
// current-definition and candidate reads check that installed ownership.
type Tool struct {
	Id             apid.ID
	Namespace      string
	Name           common.ResourceName
	ConnectionId   apid.ID
	ActiveRevision uint64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time

	// Labels contains user labels plus derived apxy/ labels. Standalone writes
	// regenerate the system portion instead of trusting supplied projections.
	Labels Labels
	// Annotations belong to the Tool; parent annotations are never inherited.
	Annotations Annotations

	// Description is the active definition's public descriptive text. The trusted
	// preparer supplies it from the same plaintext it encrypts; this layer cannot
	// verify that correspondence. Never put resolved configuration or credentials
	// into this projection. Legacy rows may be empty until a definition update.
	Description string
	// Verbs are exact permission aliases, stored in sorted order. They support
	// candidate filtering, not authorization by themselves. Legacy nil values
	// mean unprepared, never unrestricted access. Historical aliases remain in
	// the encrypted revision; this list always describes ActiveRevision.
	Verbs []string

	// BindingId identifies the controller assignment that owns this Tool; nil
	// means standalone. A binding's removal/recreation creates a new identity.
	BindingId apid.ID
	// SourceKey is the exact source identity within a binding, independent of
	// display name. It is empty only for standalone Tools. The pair stays unique
	// across tombstones so a later inventory can restore the original Tool ID.
	SourceKey string
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
		"labels",
		"annotations",
		"description",
		"verbs",
		"binding_id",
		"source_key",
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
		&t.Labels,
		&t.Annotations,
		&t.Description,
		(*toolVerbsDB)(&t.Verbs),
		&t.BindingId,
		&t.SourceKey,
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
		t.Labels,
		t.Annotations,
		t.Description,
		toolVerbsDB(t.Verbs),
		t.BindingId,
		t.SourceKey,
	}
}

// Validate checks identity, metadata, definition projections, and the active
// revision. Legacy rows with missing projections remain readable but must have
// them supplied before a definition write. Parent liveness and connection
// namespace equality require transactional checks.
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

	if t.BindingId.IsNil() {
		if t.SourceKey != "" {
			return errors.New("standalone tool cannot have a source key")
		}
	} else {
		if err := validateToolSetBindingID(t.BindingId); err != nil {
			return err
		}
		if strings.TrimSpace(t.SourceKey) == "" {
			return errors.New("generated tool source key is required")
		}
	}

	if err := t.Labels.Validate(); err != nil {
		return fmt.Errorf("invalid tool labels: %w", err)
	}

	if err := t.Annotations.Validate(); err != nil {
		return fmt.Errorf("invalid tool annotations: %w", err)
	}
	if err := validateToolDefinitionProjection(t.Description, t.Verbs); err != nil {
		return err
	}

	return validateToolRevision(t.ActiveRevision)
}

// requireStandalone rejects any supplied or persisted ownership, including an
// incomplete pair. Callers must check the stored row as well as input so clearing
// ownership on an update cannot bypass the controller's authority.
func (t *Tool) requireStandalone() error {
	if !t.BindingId.IsNil() || t.SourceKey != "" {
		return fmt.Errorf("%w: binding %s", ErrToolManaged, t.BindingId)
	}
	return nil
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

// populateToolLabels checks the exact connection namespace and derives labels
// from parent metadata and the Tool's authored labels. Identity labels come from
// actual columns, so supplied or stale projections cannot shadow the binding.
// Authoring requires a live connection; reconciliation also projects deleted
// connections while their Tools remain visible for management. Neither path
// reads credentials, configuration, or parent annotations. Parent reads do not
// take write locks; the caller must protect the Tool from concurrent changes.
func (s *service) populateToolLabels(
	ctx context.Context,
	tx *sql.Tx,
	tool *Tool,
	requireLiveConnection bool,
) error {
	var ns string
	var connectionName common.ResourceName
	var connectionLabels, namespaceLabels Labels
	query := s.sq.Select(
		"c.namespace",
		"c.name",
		"c.labels",
		"n.labels",
	).
		From(ConnectionsTable + " c").
		Join(NamespacesTable + " n ON n.path = c.namespace").
		Where(sq.Eq{
			"c.id":         tool.ConnectionId,
			"n.deleted_at": nil,
		})
	if requireLiveConnection {
		query = query.Where(sq.Eq{"c.deleted_at": nil})
	}
	err := query.
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&ns, &connectionName, &connectionLabels, &namespaceLabels)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if ns != tool.Namespace {
		return errors.New("tool and connection must have the same namespace")
	}

	userLabels, _ := SplitUserAndApxyLabels(tool.Labels)
	// The namespace is read directly, so discard its cached projection on the
	// connection. Otherwise removed labels could survive forever on Tools whose
	// soft-deleted connection no longer participates in parent reconciliation.
	for key := range connectionLabels {
		if strings.HasPrefix(key, meta.SystemLabelPrefix+NamespaceLabelToken+"/") {
			delete(connectionLabels, key)
		}
	}
	connectionLabels = InjectSelfImplicitLabels(
		tool.ConnectionId,
		connectionName,
		ns,
		connectionLabels,
	)
	tool.Labels = ApplyParentCarryForward(
		userLabels,
		ParentCarryForward{
			Rt:     NamespaceLabelToken,
			Labels: namespaceLabels,
		},
		ParentCarryForward{
			Rt:     ApidPrefixToLabelToken(apid.PrefixConnection),
			Labels: connectionLabels,
		},
	)
	// Parent identities must describe the actual binding even if the namespace's
	// own stored identity projection has not yet been reconciled.
	tool.Labels = InjectNamespaceSelfImplicitLabels(ns, tool.Labels)
	tool.Labels = InjectSelfImplicitLabels(tool.Id, tool.Name, tool.Namespace, tool.Labels)

	return nil
}

// lockToolForWrite protects a live Tool before reading its current binding and
// metadata. Authoring takes connection locks first. Label reconciliation takes
// only this lock and reads parents without locking them. PostgreSQL uses a row
// lock: a neutral UPDATE followed by a label UPDATE would force foreign-key
// rechecks against parents and could reverse authoring's connection-to-Tool lock
// order. SQLite needs an initial write to avoid upgrading a read snapshot.
func (s *service) lockToolForWrite(
	ctx context.Context,
	tx *sql.Tx,
	id apid.ID,
) error {
	if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
		var lockedID apid.ID
		err := s.sq.Select("id").
			From(ToolsTable).
			Where(sq.Eq{
				"id":         id,
				"deleted_at": nil,
			}).
			Suffix("FOR UPDATE").
			RunWith(tx).
			QueryRowContext(ctx).
			Scan(&lockedID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}

		return err
	}

	result, err := s.sq.Update(ToolsTable).
		Set("id", id).
		Where(sq.Eq{
			"id":         id,
			"deleted_at": nil,
		}).
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

	return nil
}

// toolStorageTime returns the injected clock's time at microsecond precision,
// the finest both PostgreSQL and SQLite preserve. Tool and ToolSet writes return
// records built in memory, so a finer value would make those responses disagree
// with later reads and fail createdAt immutability checks on PostgreSQL.
func toolStorageTime(ctx context.Context) time.Time {
	return apctx.GetClock(ctx).Now().Truncate(time.Microsecond)
}

// CreateStandaloneTool stores a new identity and its active revision together.
// The caller supplies a validated, compiled, encrypted definition and its digest;
// this layer does not decrypt or compile it. Names default to the allocated ID.
// Only a successful commit changes the caller's record. Existing IDs, including
// tombstones, are never revived; duplicate live namespace/name pairs conflict.
// Metadata maps are detached and system labels are derived from the parents.
// Description and Verbs must come from the same prepared plaintext definition;
// verbs are copied and sorted without changing the caller's input slice.
// Supplying controller ownership is rejected; generated identities are
// installed only by an atomic binding inventory operation.
func (s *service) CreateStandaloneTool(
	ctx context.Context,
	tool *Tool,
	definition encfield.EncryptedField,
	digest string,
) (*ToolRevision, error) {
	if tool == nil {
		return nil, errors.New("tool is required")
	}
	if err := tool.requireStandalone(); err != nil {
		return nil, err
	}

	candidate := *tool
	candidate.Labels = tool.Labels.Copy()
	candidate.Annotations = tool.Annotations.Copy()
	candidate.Verbs = slices.Clone(tool.Verbs)
	slices.Sort(candidate.Verbs)
	if candidate.Name == "" {
		candidate.Name = common.ResourceName(candidate.Id.String())
	}
	candidate.ActiveRevision = 1
	candidate.CreatedAt = toolStorageTime(ctx)
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
	if err := s.populateToolLabels(ctx, tx, &candidate, true); err != nil {
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
		Where(sq.Eq{
			"t.id":         id,
			"t.deleted_at": nil,
			"n.deleted_at": nil,
		}).
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
// name, connection, metadata, and active pointer. The namespace cannot change.
// Every accepted update advances the revision, even for identical content. Definitions
// must already be validated and compiled by the caller. Failed or stale writes
// preserve history and leave the caller's record unchanged.
// Labels and annotations are full replacements; system labels are re-derived
// so retargeting cannot retain projections from the previous connection.
// Description and Verbs fully replace the active projection in that same
// transaction and must describe the caller's prepared encrypted definition.
// Both supplied and stored ownership are checked so an update cannot detach or
// replace a generated Tool by presenting it as standalone.
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
	if err := tool.requireStandalone(); err != nil {
		return nil, err
	}

	if err := validateToolRevision(expectedRevision); err != nil {
		return nil, err
	}

	if expectedRevision == math.MaxInt64 {
		return nil, ErrToolRevisionConflict
	}

	candidate := *tool
	candidate.Labels = tool.Labels.Copy()
	candidate.Annotations = tool.Annotations.Copy()
	candidate.Verbs = slices.Clone(tool.Verbs)
	slices.Sort(candidate.Verbs)
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

	if err := s.lockToolConnections(
		ctx,
		tx,
		observed.ConnectionId,
		candidate.ConnectionId,
	); err != nil {
		return nil, err
	}
	if err := s.lockToolForWrite(ctx, tx, candidate.Id); err != nil {
		return nil, err
	}

	current, err := s.getTool(ctx, tx, candidate.Id)
	if err != nil {
		return nil, err
	}
	if err := current.requireStandalone(); err != nil {
		return nil, err
	}

	if current.ActiveRevision != expectedRevision ||
		current.ConnectionId != observed.ConnectionId {
		return nil, ErrToolRevisionConflict
	}

	if current.Namespace != candidate.Namespace {
		return nil, errors.New("tool namespace cannot change")
	}

	if err := s.populateToolLabels(ctx, tx, &candidate, true); err != nil {
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
		Set("labels", candidate.Labels).
		Set("annotations", candidate.Annotations).
		Set("description", candidate.Description).
		Set("verbs", toolVerbsDB(candidate.Verbs)).
		Set("active_revision", candidate.ActiveRevision).
		Set("updated_at", candidate.UpdatedAt).
		Where(sq.Eq{
			"id":              candidate.Id,
			"active_revision": expectedRevision,
			"deleted_at":      nil,
			"binding_id":      nil,
		}).
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
// Controller-owned identities cannot be individually deleted by this method.
func (s *service) DeleteStandaloneTool(
	ctx context.Context,
	id apid.ID,
	expectedRevision uint64,
) error {
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
		Where(sq.Eq{
			"id":              id,
			"active_revision": expectedRevision,
			"deleted_at":      nil,
			"binding_id":      nil,
		}).
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
		current, err := s.GetTool(ctx, id)
		if err != nil {
			return err
		}
		if err := current.requireStandalone(); err != nil {
			return err
		}
		return ErrToolRevisionConflict
	}

	return nil
}
