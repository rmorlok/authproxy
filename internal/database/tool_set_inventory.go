package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

var (
	// ErrToolSetBindingInventoryConflict prevents first installation from being
	// used to replace an installed inventory or reuse retained identities, and
	// rejects installed inventory counters that cannot advance.
	ErrToolSetBindingInventoryConflict = errors.New("toolset binding inventory state conflicts")

	// ErrToolSetInventoryGenerationConflict means the target is no longer
	// eligible: first installation requires primary, replacement primary/active.
	ErrToolSetInventoryGenerationConflict = errors.New("toolset inventory generation is no longer eligible")

	// ErrToolSetInventoryConnectionNotReady preserves the prior inventory while
	// the connection is unconfigured or disconnected.
	ErrToolSetInventoryConnectionNotReady = errors.New("toolset inventory connection is not configured")
)

// PreparedToolSetTool pairs a generated identity and its discovery metadata with
// a compiled, encrypted explicit-template definition. The trusted caller supplies
// a provisional fresh ID, exact source key, name, namespace, connection, and
// binding identity. Replacement resolves retained IDs by source key; an omitted
// name retains the existing name. This layer derives parent labels, revision
// numbers, timestamps, and provenance.
// Compilation and encryption must finish before calling the database; definitions
// must retain symbolic configuration references, not resolved connection values.
type PreparedToolSetTool struct {
	Tool                Tool
	EncryptedDefinition encfield.EncryptedField
	DefinitionDigest    string
}

// ToolSetInventoryInstall captures a complete inventory prepared from a
// published explicit ToolSet generation, for first installation or replacement.
// Both nil and empty Tools install an empty inventory. Remote catalogs require
// connection-context fencing and are not supported by these explicit operations.
type ToolSetInventoryInstall struct {
	BindingId                apid.ID
	ExpectedEpoch            uint64
	ExpectedSelectorRevision uint64
	Generation               uint64
	Tools                    []PreparedToolSetTool
}

// prepare detaches and validates the complete batch without database writes or
// changes to caller-owned maps/slices. Source keys are compared exactly. The
// database still checks namespace names and IDs against all retained records.
func (p ToolSetInventoryInstall) prepare(
	ctx context.Context,
) ([]Tool, []*ToolRevision, error) {
	if err := validateToolSetBindingID(p.BindingId); err != nil {
		return nil, nil, err
	}

	if p.ExpectedEpoch == 0 || p.ExpectedEpoch >= math.MaxInt64 {
		return nil, nil, errors.New("expected binding epoch must be positive and allow a signed 64-bit increment")
	}

	if err := validateBindingSelectorRevision(p.ExpectedSelectorRevision); err != nil {
		return nil, nil, err
	}

	if p.Generation == 0 || p.Generation > math.MaxInt64 {
		return nil, nil, errors.New("toolset inventory generation must be a positive signed 64-bit integer")
	}

	tools := make([]Tool, 0, len(p.Tools))
	revisions := make([]*ToolRevision, 0, len(p.Tools))
	keys := map[string]bool{}

	for _, entry := range p.Tools {
		candidate := entry.Tool

		if candidate.BindingId != p.BindingId {
			return nil, nil, errors.New("tool inventory entry must belong to the requested binding")
		}

		candidate.Labels = candidate.Labels.Copy()
		candidate.Annotations = candidate.Annotations.Copy()
		candidate.Verbs = slices.Clone(candidate.Verbs)
		slices.Sort(candidate.Verbs)

		if candidate.Name == "" {
			candidate.Name = common.ResourceName(candidate.Id.String())
		}

		candidate.ActiveRevision = 1
		candidate.CreatedAt = apctx.GetClock(ctx).Now()
		candidate.UpdatedAt = candidate.CreatedAt
		candidate.DeletedAt = nil

		if err := candidate.Validate(); err != nil {
			return nil, nil, err
		}

		if keys[candidate.SourceKey] {
			return nil, nil, errors.New("tool inventory contains duplicate source keys")
		}
		keys[candidate.SourceKey] = true

		revision := newToolRevision(
			ctx,
			&candidate,
			entry.EncryptedDefinition,
			entry.DefinitionDigest,
		)
		generation := p.Generation
		revision.SourceGeneration = &generation
		if err := revision.Validate(); err != nil {
			return nil, nil, err
		}

		tools = append(tools, candidate)
		revisions = append(revisions, revision)
	}

	return tools, revisions, nil
}

// toolSetInventoryNamespace checks readiness and both namespace parents after
// locking the ToolSet and Connection. PostgreSQL share locks keep namespace soft
// deletion from racing this inventory write; SQLite's initial parent writes already
// serialize writers. This runs even for empty inventories. It reads no credentials
// or configuration and does not assert that a remote catalog context is current.
func (s *service) toolSetInventoryNamespace(
	ctx context.Context,
	tx *sql.Tx,
	binding *ToolSetBinding,
) (string, error) {
	query := s.sq.Select(
		"c.namespace",
		"c.state",
	).
		From(ConnectionsTable+" c").
		Join(ToolSetsTable+" t ON t.id = ?", binding.ToolSetId).
		Join(NamespacesTable + " cn ON cn.path = c.namespace").
		Join(NamespacesTable + " tn ON tn.path = t.namespace").
		Where(sq.Eq{
			"c.id":          binding.ConnectionId,
			"c.deleted_at":  nil,
			"t.deleted_at":  nil,
			"cn.deleted_at": nil,
			"tn.deleted_at": nil,
		})
	if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
		query = query.Suffix("FOR SHARE OF cn, tn")
	}

	var namespace string
	var state ConnectionState
	err := query.
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&namespace, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if state != ConnectionStateConfigured {
		return "", ErrToolSetInventoryConnectionNotReady
	}

	return namespace, nil
}

// InstallExplicitToolSetInventory installs a pending binding's complete first
// inventory and advances its epoch in one transaction. The target must remain
// primary and the captured selector, membership, and binding epoch must still
// match. The binding pins that generation; later publication does not migrate it.
// All failures preserve the prior database state and caller inputs. Generated
// discovery/current-definition reads remain disabled until admission checks land.
func (s *service) InstallExplicitToolSetInventory(
	ctx context.Context,
	params ToolSetInventoryInstall,
) (*ToolSetBinding, error) {
	return s.commitExplicitToolSetInventory(ctx, params, 0)
}

// commitExplicitToolSetInventory shares parent locking, readiness, preparation,
// and atomic publication between first installation and explicit replacement.
// expectedGeneration is zero only for the pending-to-installed transition.
func (s *service) commitExplicitToolSetInventory(
	ctx context.Context,
	params ToolSetInventoryInstall,
	expectedGeneration uint64,
) (*ToolSetBinding, error) {
	tools, revisions, err := params.prepare(ctx)
	if err != nil {
		return nil, err
	}

	// Capture immutable parent IDs, then reread the assignment after locking
	// those parents in the same order as membership creation/removal.
	identity, err := s.getToolSetBinding(
		ctx,
		s.db,
		sq.Eq{"b.id": params.BindingId},
		false, // requireLiveParents
	)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	selectorRevision, selected, live, err := s.lockToolSetBindingMembership(
		ctx,
		tx,
		identity.ToolSetId,
		identity.ConnectionId,
	)
	if err != nil {
		return nil, err
	}
	if !live {
		return nil, ErrNotFound
	}

	if selectorRevision != params.ExpectedSelectorRevision {
		return nil, ErrToolSetSelectorRevisionConflict
	}

	if !selected {
		return nil, ErrToolSetBindingMembershipConflict
	}

	binding, err := s.getToolSetBinding(ctx, tx, sq.Eq{"b.id": params.BindingId}, false)
	if err != nil {
		return nil, err
	}

	if binding.Epoch != params.ExpectedEpoch {
		return nil, ErrToolSetBindingEpochConflict
	}

	if expectedGeneration == 0 {
		if binding.AppliedGeneration != nil || binding.InventoryRevision != 0 {
			return nil, ErrToolSetBindingInventoryConflict
		}
	} else {
		if binding.AppliedGeneration == nil || *binding.AppliedGeneration != expectedGeneration {
			return nil, ErrToolSetBindingGenerationConflict
		}
		if binding.InventoryRevision == 0 || binding.InventoryRevision >= math.MaxInt64 {
			return nil, ErrToolSetBindingInventoryConflict
		}
	}

	namespace, err := s.toolSetInventoryNamespace(ctx, tx, binding)
	if err != nil {
		return nil, err
	}

	// Include tombstones so a returning source key recovers its original ID and
	// revision sequence. A pending binding may not reuse any retained inventory.
	existing, err := s.lockToolSetInventory(ctx, tx, binding.Id)
	if err != nil {
		return nil, err
	}
	if expectedGeneration == 0 && len(existing) != 0 {
		return nil, ErrToolSetBindingInventoryConflict
	}

	var state toolsetschema.ToolSetReleaseState
	err = s.sq.
		Select("state").
		From(ToolSetGenerationsTable).
		Where(sq.Eq{
			"tool_set_id": binding.ToolSetId,
			"generation":  params.Generation,
			"deleted_at":  nil,
		}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if state != toolsetschema.ToolSetReleaseStatePrimary &&
		(expectedGeneration == 0 || state != toolsetschema.ToolSetReleaseStateActive) {
		return nil, ErrToolSetInventoryGenerationConflict
	}

	if err := resolveToolSetInventoryIdentities(params, tools, revisions, existing); err != nil {
		return nil, err
	}

	// Validate every parent assignment and derive metadata before inserting any
	// definitions. SQL uniqueness failures still roll back the entire batch.
	for i := range tools {
		if tools[i].ConnectionId != binding.ConnectionId ||
			tools[i].Namespace != namespace {
			return nil, errors.New("tool inventory entry must use the binding connection and namespace")
		}

		if err := s.populateToolLabels(ctx, tx, &tools[i], true); err != nil {
			return nil, err
		}
	}

	if expectedGeneration != 0 {
		// Release all owned names before restoring the new complete inventory,
		// permitting swaps and reuse of removed names in the same transaction.
		// Already-retired rows keep their original removal timestamp and history.
		now := apctx.GetClock(ctx).Now()
		_, err = s.sq.
			Update(ToolsTable).
			Set("deleted_at", now).
			Set("updated_at", now).
			Where(sq.Eq{"binding_id": binding.Id, "deleted_at": nil}).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return nil, wrapDatabaseMutationError("failed to retire prior tool inventory", err)
		}
	}

	for i := range tools {
		previous, retained := existing[tools[i].SourceKey]
		if err := s.writeToolSetInventoryEntry(ctx, tx, &tools[i], revisions[i], previous, retained); err != nil {
			return nil, err
		}
	}

	var result ToolSetBinding
	err = s.sq.
		Update(ToolSetBindingsTable).
		Set("applied_generation", params.Generation).
		Set("inventory_revision", sq.Expr("inventory_revision + 1")).
		Set("epoch", sq.Expr("epoch + 1")).
		Set("updated_at", apctx.GetClock(ctx).Now()).
		Where(sq.Eq{
			"id":                 binding.Id,
			"epoch":              params.ExpectedEpoch,
			"applied_generation": binding.AppliedGeneration,
			"inventory_revision": binding.InventoryRevision,
			"deleted_at":         nil,
		}).
		Suffix("RETURNING " + strings.Join(result.cols(), ", ")).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrToolSetBindingEpochConflict
	}
	if err != nil {
		return nil, fmt.Errorf("failed to install toolset inventory: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDatabaseMutationError("failed to commit toolset inventory", err)
	}

	return &result, nil
}
