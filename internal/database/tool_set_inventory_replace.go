package database

import (
	"context"
	"database/sql"
	"errors"
	"math"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// ErrToolSetBindingGenerationConflict means replacement did not observe the
// installed generation captured during preparation, including a pending binding.
var ErrToolSetBindingGenerationConflict = errors.New("toolset binding applied generation changed")

// ReplaceExplicitToolSetInventory atomically replaces a complete installed
// inventory with a prepared primary or active generation of the same ToolSet.
// expectedGeneration is the applied generation captured before preparation;
// the request also fences the binding epoch and current selector revision.
// Surviving and restored source keys preserve their IDs and creation times.
// Explicit names and metadata replace their prior values; omitted names retain
// the stored name. Every included key gets a new immutable revision, even for
// identical content or a same-generation replacement. Absent keys are retired,
// retaining identities and history for later restoration. Empty inventories are
// valid. Failures preserve the entire prior inventory and all caller inputs.
// This operation does not perform compilation, authorize migration, or support
// remote catalogs; their connection-context fencing belongs to a later layer.
func (s *service) ReplaceExplicitToolSetInventory(
	ctx context.Context,
	params ToolSetInventoryInstall,
	expectedGeneration uint64,
) (*ToolSetBinding, error) {
	if expectedGeneration == 0 || expectedGeneration > math.MaxInt64 {
		return nil, errors.New("expected applied generation must be a positive signed 64-bit integer")
	}

	return s.commitExplicitToolSetInventory(ctx, params, expectedGeneration)
}

// lockToolSetInventory reads all owned identities, including tombstones, after
// locking the binding's parents. Row locks serialize PostgreSQL label repair
// with replacement; SQLite already serializes writers through the parent locks.
// Source keys remain exact, without normalization or database collation matching.
func (s *service) lockToolSetInventory(
	ctx context.Context,
	tx *sql.Tx,
	bindingID apid.ID,
) (map[string]Tool, error) {
	var record Tool
	query := s.sq.
		Select(record.cols()...).
		From(ToolsTable).
		Where(sq.Eq{"binding_id": bindingID}).
		OrderBy("id")
	if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
		query = query.Suffix("FOR UPDATE")
	}

	rows, err := query.
		RunWith(tx).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]Tool)
	for rows.Next() {
		var tool Tool
		if err := rows.Scan(tool.fields()...); err != nil {
			return nil, err
		}
		result[tool.SourceKey] = tool
	}

	return result, rows.Err()
}

// resolveToolSetInventoryIdentities adopts retained IDs before projecting labels
// or writing revisions. Candidate IDs are used only for previously unseen keys.
// Name/ID uniqueness is checked again because defaults and retained identities
// can differ from the caller's provisional values. The slices are detached by
// prepare; this helper never changes caller-owned metadata.
func resolveToolSetInventoryIdentities(
	params ToolSetInventoryInstall,
	tools []Tool,
	revisions []*ToolRevision,
	existing map[string]Tool,
) error {
	ids, names := map[apid.ID]bool{}, map[common.ResourceName]bool{}
	for i := range tools {
		tool := &tools[i]
		if previous, ok := existing[tool.SourceKey]; ok {
			if previous.ActiveRevision >= math.MaxInt64 {
				return ErrToolRevisionConflict
			}

			tool.Id = previous.Id
			tool.CreatedAt = previous.CreatedAt
			tool.ActiveRevision = previous.ActiveRevision + 1
			if params.Tools[i].Tool.Name == "" {
				tool.Name = previous.Name
			}

			revisions[i].ToolId = tool.Id
			revisions[i].Revision = tool.ActiveRevision
		}

		if ids[tool.Id] || names[tool.Name] {
			return errors.New("tool inventory contains duplicate resolved ids or names")
		}

		ids[tool.Id], names[tool.Name] = true, true
	}

	return nil
}

// writeToolSetInventoryEntry appends a prepared revision and publishes its
// identity/projections in the enclosing inventory transaction. Retained rows may
// be tombstones; their binding, source key, namespace, connection and creation
// time remain immutable. New identities rely on the deferred active-revision FK.
func (s *service) writeToolSetInventoryEntry(
	ctx context.Context,
	tx *sql.Tx,
	tool *Tool,
	revision *ToolRevision,
	previous Tool,
	retained bool,
) error {
	if !retained {
		_, err := s.sq.
			Insert(ToolsTable).
			Columns(tool.cols()...).
			Values(tool.values()...).
			RunWith(tx).
			ExecContext(ctx)
		if err != nil {
			return wrapDatabaseMutationError("failed to install tool identity", err)
		}
	}
	if err := s.insertToolRevision(ctx, tx, revision); err != nil {
		return err
	}
	if !retained {
		return nil
	}

	result, err := s.sq.
		Update(ToolsTable).
		Set("name", tool.Name).
		Set("labels", tool.Labels).
		Set("annotations", tool.Annotations).
		Set("description", tool.Description).
		Set("verbs", toolVerbsDB(tool.Verbs)).
		Set("active_revision", tool.ActiveRevision).
		Set("updated_at", tool.UpdatedAt).
		Set("deleted_at", nil).
		Where(sq.Eq{
			"id":              tool.Id,
			"binding_id":      tool.BindingId,
			"source_key":      tool.SourceKey,
			"active_revision": previous.ActiveRevision,
		}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return wrapDatabaseMutationError("failed to replace tool identity", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrToolRevisionConflict
	}

	return nil
}
