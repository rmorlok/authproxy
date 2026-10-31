package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"golang.org/x/time/rate"
)

// Carry-forward propagation
// =========================
//
// When a parent's user labels change, the materialized apxy/ portion of
// every descendant resource needs to be re-derived. This file contains the
// helpers that do that work.
//
// All public entry points (RefreshNamespaceLabelsCarryForward,
// RefreshConnectionsForConnector) drive the walk WITHOUT holding a
// long-running transaction — each row's update runs in its own short
// transaction so concurrent reads are not blocked. Some intermediate state
// drift between rows during a propagation pass is acceptable; the daily
// consistency checker (see issue #198) is the safety net.
//
// These methods are intended to be invoked from background asynq tasks
// rather than from synchronous API calls — a label change on a deeply
// nested namespace can fan out to a large number of descendants and a
// foreground propagation would block the originating request for too long.

// RefreshNamespaceLabelsCarryForward re-derives the materialized apxy/
// portion of every resource that inherits from nsPath, then walks each
// direct child namespace, recomputes its own labels, and recurses. Each
// row's update runs in its own short transaction.
//
// nsPath itself is NOT touched — the caller is responsible for having
// already written the new user labels on that row.
func (s *service) RefreshNamespaceLabelsCarryForward(ctx context.Context, nsPath string) error {
	if err := s.refreshConnectionsInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshActorsInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshEncryptionKeysInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshConnectorsInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshRateLimitsInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshToolSetsInNamespace(ctx, nsPath); err != nil {
		return err
	}
	if err := s.refreshToolsInNamespace(ctx, nsPath); err != nil {
		return err
	}

	childPaths, err := s.directChildNamespacePaths(ctx, nsPath)
	if err != nil {
		return err
	}
	for _, childPath := range childPaths {
		if _, err := s.recomputeNamespaceLabelsTx(ctx, childPath); err != nil {
			return err
		}
		if err := s.RefreshNamespaceLabelsCarryForward(ctx, childPath); err != nil {
			return err
		}
	}
	return nil
}

// RefreshConnectionsForConnector re-derives the materialized apxy/
// portion of every connection pointing at the logical connector. Labels are
// connector-owned, so a change applies across every generation.
func (s *service) RefreshConnectionsForConnector(ctx context.Context, id apid.ID) error {
	rows, err := s.sq.
		Select("id").
		From(ConnectionsTable).
		Where(sq.Eq{
			"connector_id": id,
			"deleted_at":   nil,
		}).
		RunWith(s.db).
		Query()
	if err != nil {
		return err
	}
	var ids []apid.ID
	for rows.Next() {
		var connID apid.ID
		if err := rows.Scan(&connID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, connID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, connID := range ids {
		if _, err := s.recomputeConnectionLabelsTx(ctx, connID); err != nil {
			return err
		}
		// Tool projections can drift even when the connection is already correct.
		if err := s.refreshToolsForConnection(ctx, connID); err != nil {
			return err
		}
	}
	return nil
}

// directChildNamespacePaths returns the paths of all immediate (depth+1)
// non-deleted child namespaces of nsPath.
func (s *service) directChildNamespacePaths(ctx context.Context, nsPath string) ([]string, error) {
	depth := namespace.DepthOfPath(nsPath) + 1
	rows, err := s.sq.
		Select("path").
		From(NamespacesTable).
		Where(sq.And{
			sq.Like{"path": nsPath + namespace.PathSeparator + "%"},
			sq.Eq{"depth": depth, "deleted_at": nil},
		}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// scanIdsByNamespace returns the ids of every non-deleted row whose
// namespace column equals nsPath. Read-only — runs against s.db.
func (s *service) scanIdsByNamespace(table, nsPath string) ([]apid.ID, error) {
	rows, err := s.sq.
		Select("id").
		From(table).
		Where(sq.Eq{"namespace": nsPath, "deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []apid.ID
	for rows.Next() {
		var id apid.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *service) refreshConnectionsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(ConnectionsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeConnectionLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) refreshActorsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(ActorTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeActorLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) refreshEncryptionKeysInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(KeysTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeEncryptionKeyLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) refreshRateLimitsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(RateLimitsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeRateLimitLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) refreshConnectorsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(ConnectorsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeConnectorLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshToolSetsInNamespace refreshes each live ToolSet in the exact namespace.
// Per-row transactions preserve the existing propagation consistency model.
func (s *service) refreshToolSetsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(ToolSetsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeToolSetLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshToolsInNamespace repairs Tools after their namespace's connections have
// been refreshed. Each Tool is re-read and updated in its own short transaction.
func (s *service) refreshToolsInNamespace(ctx context.Context, nsPath string) error {
	ids, err := s.scanIdsByNamespace(ToolsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.recomputeToolLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshToolsForConnection repairs descendants of one refreshed connection.
// Re-reading each Tool under its write lock makes concurrent retargeting safe.
func (s *service) refreshToolsForConnection(
	ctx context.Context,
	connectionID apid.ID,
) error {
	rows, err := s.sq.
		Select("id").
		From(ToolsTable).
		Where(sq.Eq{
			"connection_id": connectionID,
			"deleted_at":    nil,
		}).
		RunWith(s.db).
		QueryContext(ctx)
	if err != nil {
		return err
	}

	var ids []apid.ID
	for rows.Next() {
		var id apid.ID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}

	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}

	for _, id := range ids {
		if _, err := s.recomputeToolLabelsTx(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// recomputeConnectionLabelsTx opens a short transaction, re-derives the
// connection's full labels from its current connector generation + namespace +
// own user labels, and persists the result if it differs. Returns true if
// drift was detected and corrected.
func (s *service) recomputeConnectionLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var conn Connection
		err := s.sq.
			Select(conn.cols()...).
			From(ConnectionsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(conn.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(conn.Labels)

		cvLabels, err := s.fetchLabelsForCarryForward(ctx, tx, ConnectorsTable, sq.Eq{
			"id":         conn.ConnectorId,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       conn.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: ApidPrefixToLabelToken(apid.PrefixConnector), Labels: cvLabels},
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(conn.Id, conn.Name, conn.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, conn.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// recomputeActorLabelsTx opens a short transaction and re-derives an
// actor's full labels from its namespace and own user labels. Returns true
// if drift was detected and corrected.
func (s *service) recomputeActorLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var a Actor
		err := s.sq.
			Select(a.cols()...).
			From(ActorTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(a.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(a.Labels)
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       a.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(a.Id, a.Name, a.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, a.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// recomputeEncryptionKeyLabelsTx opens a short transaction and re-derives
// an encryption key's full labels from its namespace and own user labels.
// Returns true if drift was detected and corrected.
func (s *service) recomputeEncryptionKeyLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var ek Key
		err := s.sq.
			Select(ek.cols()...).
			From(KeysTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(ek.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(ek.Labels)
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       ek.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(ek.Id, ek.Name, ek.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, KeysTable, sq.Eq{"id": id, "deleted_at": nil}, ek.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// recomputeRateLimitLabelsTx opens a short transaction and re-derives a
// rate limit's full labels from its namespace and own user labels. Returns
// true if drift was detected and corrected.
func (s *service) recomputeRateLimitLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var rl RateLimit
		err := s.sq.
			Select(rl.cols()...).
			From(RateLimitsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(rl.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(rl.Labels)
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       rl.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(rl.Id, rl.Name, rl.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, RateLimitsTable, sq.Eq{"id": id, "deleted_at": nil}, rl.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// recomputeConnectorLabelsTx opens a short transaction and
// re-derives a connector's full labels from its namespace and own
// user labels. Returns true if drift was detected and corrected.
func (s *service) recomputeConnectorLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var connector Connector
		err := s.sq.
			Select(connector.cols()...).
			From(ConnectorsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(connector.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(connector.Labels)
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       connector.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(connector.Id, connector.Name, connector.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(
			ctx,
			tx,
			ConnectorsTable,
			sq.Eq{"id": id, "deleted_at": nil},
			connector.Labels,
			newLabels,
		)
		return werr
	})
	return corrected, err
}

// recomputeToolSetLabelsTx re-derives namespace and self labels in a short
// transaction. Only drifted labels and updated_at are written; selector state
// and its revision remain unchanged. Concurrent deletion is a harmless no-op.
func (s *service) recomputeToolSetLabelsTx(ctx context.Context, id apid.ID) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	toolSet, err := s.getToolSet(ctx, tx, sq.Eq{"id": id})
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	userLabels, _ := SplitUserAndApxyLabels(toolSet.Labels)

	nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
		"path": toolSet.Namespace, "deleted_at": nil,
	})
	if err != nil {
		return false, err
	}

	newLabels := ApplyParentCarryForward(userLabels, ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels})
	newLabels = InjectSelfImplicitLabels(toolSet.Id, toolSet.Name, toolSet.Namespace, newLabels)

	corrected, err := s.writeRecomputedLabels(ctx, tx, ToolSetsTable, sq.Eq{"id": id, "deleted_at": nil}, toolSet.Labels, newLabels)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return corrected, nil
}

// recomputeToolLabelsTx rebuilds projections from the Tool's current binding and
// authored labels. Locking before reading prevents a sweep from restoring stale
// labels after a rename, retarget, or metadata edit. Parent label changes remain
// eventually consistent. Only labels and updated_at change; definitions, active
// revisions, and annotations are untouched. A deleted Tool/namespace is skipped.
func (s *service) recomputeToolLabelsTx(
	ctx context.Context,
	id apid.ID,
) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if err := s.lockToolForWrite(ctx, tx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	tool, err := s.getTool(ctx, tx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	previous := tool.Labels
	if err := s.populateToolLabels(
		ctx,
		tx,
		tool,
		false, // requireLiveConnection
	); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	corrected, err := s.writeRecomputedLabels(
		ctx,
		tx,
		ToolsTable,
		sq.Eq{
			"id":         id,
			"deleted_at": nil,
		},
		previous,
		tool.Labels,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}

	return corrected, nil
}

// recomputeNamespaceLabelsTx opens a short transaction and re-derives a
// namespace's full labels from its immediate parent and its own user labels.
// Returns true if drift was detected and corrected.
func (s *service) recomputeNamespaceLabelsTx(ctx context.Context, path string) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var ns Namespace
		err := s.sq.
			Select(ns.cols()...).
			From(NamespacesTable).
			Where(sq.Eq{"path": path, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(ns.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(ns.Labels)

		prefixes := namespace.SplitPathToPrefixes(ns.Path)
		var parentLabels Labels
		if len(prefixes) > 1 {
			parentLabels, err = s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
				"path":       prefixes[len(prefixes)-2],
				"deleted_at": nil,
			})
			if err != nil {
				return err
			}
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: parentLabels},
		)
		newLabels = InjectNamespaceSelfImplicitLabels(ns.Path, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, NamespacesTable, sq.Eq{"path": path, "deleted_at": nil}, ns.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// writeRecomputedLabels persists a recomputed labels map to a row when it
// differs from the row's current labels. Returns true if a row was actually
// updated (drift was detected and corrected). Used by both the propagation
// tasks (which expect drift after a parent change) and the daily
// reconciler (which mostly finds none).
func (s *service) writeRecomputedLabels(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, current, newLabels Labels) (bool, error) {
	if labelsEqual(current, newLabels) {
		return false, nil
	}
	_, err := s.sq.
		Update(table).
		Set("labels", newLabels).
		Set("updated_at", apctx.GetClock(ctx).Now()).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return false, fmt.Errorf("failed to write recomputed labels in %s: %w", table, err)
	}
	return true, nil
}

// labelsEqual reports whether two label maps contain exactly the same
// key/value pairs. nil and empty are treated as equal.
func labelsEqual(a, b Labels) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// reconcileCarryForwardLabelsCheckpoint names the daily sweep's task checkpoint.
const reconcileCarryForwardLabelsCheckpoint = "reconcile_carry_forward_labels"

// reconcileStep is one table walked by the carry-forward labels sweep.
type reconcileStep struct {
	// name identifies the step in logs and in the persisted checkpoint, so it must
	// stay stable across releases.
	name string
	// table is the known table to walk.
	table string
	// keyColumn is the table's unique, immutable key. Label writes never change it,
	// which keeps the walk's position valid while it updates rows.
	keyColumn string
	// recompute repairs the row with the given key, reporting whether it had drifted.
	recompute func(ctx context.Context, key string) (bool, error)
}

// byID adapts an ID-keyed recompute function to the string keys of reconcileStep.
func byID(recompute func(context.Context, apid.ID) (bool, error)) func(context.Context, string) (bool, error) {
	return func(ctx context.Context, key string) (bool, error) {
		return recompute(ctx, apid.ID(key))
	}
}

// reconcileSteps lists the sweep's walks in order: top-down, so child resources
// see parent labels repaired earlier in the same pass. Append new steps at a
// position that preserves this, keeping the names of existing steps.
func (s *service) reconcileSteps() []reconcileStep {
	return []reconcileStep{
		// Namespace paths sort each parent before its children, so a single pass
		// repairs a whole subtree.
		{name: "namespaces", table: NamespacesTable, keyColumn: "path", recompute: s.recomputeNamespaceLabelsTx},
		{name: "connectors", table: ConnectorsTable, keyColumn: "id", recompute: byID(s.recomputeConnectorLabelsTx)},
		{name: "connections", table: ConnectionsTable, keyColumn: "id", recompute: byID(s.recomputeConnectionLabelsTx)},
		{name: "actors", table: ActorTable, keyColumn: "id", recompute: byID(s.recomputeActorLabelsTx)},
		{name: "keys", table: KeysTable, keyColumn: "id", recompute: byID(s.recomputeEncryptionKeyLabelsTx)},
		{name: "rate_limits", table: RateLimitsTable, keyColumn: "id", recompute: byID(s.recomputeRateLimitLabelsTx)},
		{name: "tool_sets", table: ToolSetsTable, keyColumn: "id", recompute: byID(s.recomputeToolSetLabelsTx)},
		// Tools follow their parent connections, and this step also initializes
		// labels on rows created before metadata storage was introduced.
		{name: "tools", table: ToolsTable, keyColumn: "id", recompute: byID(s.recomputeToolLabelsTx)},
	}
}

// ReconcileCarryForwardLabels walks every labelled resource type in
// batches and re-derives the materialized apxy/ portion of each row. Each
// row's update runs in its own short transaction; matching labels and modification
// timestamps remain unchanged.
//
// Progress is saved in a task checkpoint after each full batch and each finished
// step. A sweep is finished only when its last step completes: until then, every
// call continues from the checkpoint, so a retry, a restarted worker, or the next
// scheduled run repeats at most one batch and a sweep too large for one run still
// reaches every table. Returns the number of rows corrected by this call, not the
// sweep as a whole, including when it returns an error.
//
// Overlapping calls are safe but wasteful: rows are repaired idempotently and
// the checkpoint only tracks whichever call saved last.
//
// batchSize controls the page size for each key query. limiter, when
// non-nil, is consulted before processing each row — providing a per-row
// records/sec rate limit so a daily sweep over a large fleet is bounded
// even though page reads are cheap. A nil limiter means unlimited.
func (s *service) ReconcileCarryForwardLabels(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	steps := s.reconcileSteps()
	firstStep, afterKey, err := s.reconcileResumePoint(ctx, steps)
	if err != nil {
		return 0, fmt.Errorf("load reconcile checkpoint: %w", err)
	}

	s.logger.Info(
		"starting carry-forward labels reconciliation",
		"batch_size", batchSize,
		"rate_limit", rateLimitLogValue(limiter),
		"step", steps[firstStep].name,
		"after_key", afterKey,
	)

	var totalCorrected int64

	for i := firstStep; i < len(steps); i++ {
		step := steps[i]
		saveProgress := func(lastKey string) error {
			return s.saveTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint, step.name, lastKey)
		}

		corrected, err := s.reconcileLabelsByKey(ctx, step, afterKey, batchSize, limiter, saveProgress)
		totalCorrected += corrected
		if err != nil {
			s.logger.Error("carry-forward labels reconciliation failed", "step", step.name, "corrected_so_far", totalCorrected, "error", err)
			return totalCorrected, fmt.Errorf("reconcile %s: %w", step.name, err)
		}
		s.logger.Info("reconciled resource type", "step", step.name, "corrected", corrected)

		afterKey = ""
		if i+1 < len(steps) {
			if err := s.saveTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint, steps[i+1].name, ""); err != nil {
				return totalCorrected, fmt.Errorf("save reconcile checkpoint: %w", err)
			}
		}
	}

	if err := s.deleteTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint); err != nil {
		return totalCorrected, fmt.Errorf("clear reconcile checkpoint: %w", err)
	}

	s.logger.Info("carry-forward labels reconciliation complete", "total_corrected", totalCorrected)
	return totalCorrected, nil
}

// reconcileResumePoint returns the index of the step to run first and the key
// after which to continue it. With no checkpoint the sweep starts from the top,
// as it does for a checkpoint naming a step this release no longer has.
func (s *service) reconcileResumePoint(ctx context.Context, steps []reconcileStep) (int, string, error) {
	checkpoint, err := s.getTaskCheckpoint(ctx, reconcileCarryForwardLabelsCheckpoint)
	if err != nil || checkpoint == nil {
		return 0, "", err
	}

	for i, step := range steps {
		if step.name == checkpoint.Step {
			return i, checkpoint.LastKey, nil
		}
	}

	s.logger.Warn("ignoring reconcile checkpoint for unknown step", "step", checkpoint.Step)
	return 0, "", nil
}

func rateLimitLogValue(limiter *rate.Limiter) string {
	if limiter == nil {
		return "unlimited"
	}
	return fmt.Sprintf("%v rps", limiter.Limit())
}

// reconcileLabelsByKey walks a step's live keys in bounded ascending pages,
// starting after afterKey (empty starts at the beginning). Paging by key instead
// of by offset keeps writes from moving the walk's position: PostgreSQL stores an
// updated row at a new physical location, so an unordered offset walk that repairs
// rows skips and repeats others. The caller supplies a positive batchSize; limiting
// applies per row. saveProgress runs after each full page with its last key.
// Returns the number of rows corrected, including when it returns an error.
func (s *service) reconcileLabelsByKey(
	ctx context.Context,
	step reconcileStep,
	afterKey string,
	batchSize int32,
	limiter *rate.Limiter,
	saveProgress func(lastKey string) error,
) (int64, error) {
	var corrected int64
	lastKey := afterKey
	for {
		query := s.sq.
			Select(step.keyColumn).
			From(step.table).
			Where(sq.Eq{"deleted_at": nil}).
			OrderBy(step.keyColumn).
			Limit(uint64(batchSize))
		if lastKey != "" {
			query = query.Where(sq.Gt{step.keyColumn: lastKey})
		}

		rows, err := query.
			RunWith(s.db).
			QueryContext(ctx)
		if err != nil {
			return corrected, err
		}

		keys := make([]string, 0, batchSize)
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return corrected, err
			}

			keys = append(keys, key)
		}

		// Release the read cursor before opening any per-row write transaction.
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return corrected, err
		}

		for _, key := range keys {
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return corrected, err
				}
			}

			wasCorrected, err := step.recompute(ctx, key)
			if err != nil {
				return corrected, err
			}

			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", step.name, "key", key)
			}
		}

		if len(keys) < int(batchSize) {
			return corrected, nil
		}

		lastKey = keys[len(keys)-1]
		if err := saveProgress(lastKey); err != nil {
			return corrected, fmt.Errorf("save reconcile checkpoint: %w", err)
		}
	}
}
