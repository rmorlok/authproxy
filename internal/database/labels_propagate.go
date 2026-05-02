package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util/pagination"
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
// RefreshConnectionsForConnectorVersion) drive the walk WITHOUT holding a
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
	if err := s.refreshConnectorVersionsInNamespace(ctx, nsPath); err != nil {
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

// RefreshConnectionsForConnectorVersion re-derives the materialized apxy/
// portion of every connection pointing at the given (id, version). Each
// connection's update runs in its own short transaction.
func (s *service) RefreshConnectionsForConnectorVersion(ctx context.Context, id apid.ID, version uint64) error {
	rows, err := s.sq.
		Select("id").
		From(ConnectionsTable).
		Where(sq.Eq{
			"connector_id":      id,
			"connector_version": version,
			"deleted_at":        nil,
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
	}
	return nil
}

// directChildNamespacePaths returns the paths of all immediate (depth+1)
// non-deleted child namespaces of nsPath.
func (s *service) directChildNamespacePaths(ctx context.Context, nsPath string) ([]string, error) {
	depth := DepthOfNamespacePath(nsPath) + 1
	rows, err := s.sq.
		Select("path").
		From(NamespacesTable).
		Where(sq.And{
			sq.Like{"path": nsPath + aschema.NamespacePathSeparator + "%"},
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
	ids, err := s.scanIdsByNamespace(EncryptionKeysTable, nsPath)
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

func (s *service) refreshConnectorVersionsInNamespace(ctx context.Context, nsPath string) error {
	rows, err := s.sq.
		Select("id", "version").
		From(ConnectorVersionsTable).
		Where(sq.Eq{"namespace": nsPath, "deleted_at": nil}).
		RunWith(s.db).
		Query()
	if err != nil {
		return err
	}
	type cvRef struct {
		id      apid.ID
		version uint64
	}
	var refs []cvRef
	for rows.Next() {
		var ref cvRef
		if err := rows.Scan(&ref.id, &ref.version); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, ref := range refs {
		if _, err := s.recomputeConnectorVersionLabelsTx(ctx, ref.id, ref.version); err != nil {
			return err
		}
	}
	return nil
}

// recomputeConnectionLabelsTx opens a short transaction, re-derives the
// connection's full labels from its current connector version + namespace +
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

		cvLabels, err := s.fetchLabelsForCarryForward(ctx, tx, ConnectorVersionsTable, sq.Eq{
			"id":         conn.ConnectorId,
			"version":    conn.ConnectorVersion,
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
			ParentCarryForward{Rt: ApidPrefixToLabelToken(apid.PrefixConnectorVersion), Labels: cvLabels},
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(conn.Id, conn.Namespace, newLabels)

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
		newLabels = InjectSelfImplicitLabels(a.Id, a.Namespace, newLabels)

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
		var ek EncryptionKey
		err := s.sq.
			Select(ek.cols()...).
			From(EncryptionKeysTable).
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
		newLabels = InjectSelfImplicitLabels(ek.Id, ek.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, ek.Labels, newLabels)
		return werr
	})
	return corrected, err
}

// recomputeConnectorVersionLabelsTx opens a short transaction and
// re-derives a connector version's full labels from its namespace and own
// user labels. Returns true if drift was detected and corrected.
func (s *service) recomputeConnectorVersionLabelsTx(ctx context.Context, id apid.ID, version uint64) (bool, error) {
	var corrected bool
	err := s.transaction(func(tx *sql.Tx) error {
		var cv ConnectorVersion
		err := s.sq.
			Select(cv.cols()...).
			From(ConnectorVersionsTable).
			Where(sq.Eq{"id": id, "version": version, "deleted_at": nil}).
			RunWith(tx).
			QueryRow().
			Scan(cv.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		userLabels, _ := SplitUserAndApxyLabels(cv.Labels)
		nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
			"path":       cv.Namespace,
			"deleted_at": nil,
		})
		if err != nil {
			return err
		}

		newLabels := ApplyParentCarryForward(
			userLabels,
			ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
		)
		newLabels = InjectSelfImplicitLabels(cv.Id, cv.Namespace, newLabels)

		var werr error
		corrected, werr = s.writeRecomputedLabels(ctx, tx, ConnectorVersionsTable, sq.Eq{
			"id":         id,
			"version":    version,
			"deleted_at": nil,
		}, cv.Labels, newLabels)
		return werr
	})
	return corrected, err
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

		prefixes := SplitNamespacePathToPrefixes(ns.Path)
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

// ReconcileCarryForwardLabels walks every labelled resource type in
// batches and re-derives the materialized apxy/ portion of each row. Each
// row's update runs in its own short transaction; rows whose computed
// labels match the stored labels are not rewritten. Returns the total
// number of rows corrected across all resource types.
//
// batchSize controls the page size for each list query. limiter, when
// non-nil, is consulted before processing each row — providing a per-row
// records/sec rate limit so a daily sweep over a large fleet is bounded
// even though page reads are cheap. A nil limiter means unlimited.
func (s *service) ReconcileCarryForwardLabels(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	s.logger.Info("starting carry-forward labels reconciliation", "batch_size", batchSize, "rate_limit", rateLimitLogValue(limiter))

	var totalCorrected int64

	steps := []struct {
		name string
		walk func(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error)
	}{
		// Top-down: namespaces first (so child resources see fresh
		// parent labels in subsequent passes), then per-resource walks.
		{name: "namespaces", walk: s.reconcileNamespaces},
		{name: "connector_versions", walk: s.reconcileConnectorVersions},
		{name: "connections", walk: s.reconcileConnections},
		{name: "actors", walk: s.reconcileActors},
		{name: "encryption_keys", walk: s.reconcileEncryptionKeys},
	}

	for _, step := range steps {
		corrected, err := step.walk(ctx, batchSize, limiter)
		if err != nil {
			s.logger.Error("carry-forward labels reconciliation failed", "step", step.name, "corrected_so_far", totalCorrected, "error", err)
			return totalCorrected, fmt.Errorf("reconcile %s: %w", step.name, err)
		}
		totalCorrected += corrected
		s.logger.Info("reconciled resource type", "step", step.name, "corrected", corrected)
	}

	s.logger.Info("carry-forward labels reconciliation complete", "total_corrected", totalCorrected)
	return totalCorrected, nil
}

func rateLimitLogValue(limiter *rate.Limiter) string {
	if limiter == nil {
		return "unlimited"
	}
	return fmt.Sprintf("%v rps", limiter.Limit())
}

func (s *service) reconcileNamespaces(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var corrected int64
	err := pagination.EnumerateThrottled(
		ctx,
		s.ListNamespacesBuilder().Limit(batchSize).Enumerate,
		limiter,
		func(ns Namespace) error {
			wasCorrected, err := s.recomputeNamespaceLabelsTx(ctx, ns.Path)
			if err != nil {
				return err
			}
			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", "namespace", "path", ns.Path)
			}
			return nil
		})
	return corrected, err
}

func (s *service) reconcileConnectorVersions(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var corrected int64
	err := pagination.EnumerateThrottled(
		ctx,
		s.ListConnectorVersionsBuilder().Limit(batchSize).Enumerate,
		limiter,
		func(cv ConnectorVersion) error {
			wasCorrected, err := s.recomputeConnectorVersionLabelsTx(ctx, cv.Id, cv.Version)
			if err != nil {
				return err
			}
			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", "connector_version", "id", cv.Id, "version", cv.Version)
			}
			return nil
		})
	return corrected, err
}

func (s *service) reconcileConnections(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var corrected int64
	err := pagination.EnumerateThrottled(
		ctx,
		s.ListConnectionsBuilder().Limit(batchSize).Enumerate,
		limiter,
		func(conn Connection) error {
			wasCorrected, err := s.recomputeConnectionLabelsTx(ctx, conn.Id)
			if err != nil {
				return err
			}
			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", "connection", "id", conn.Id)
			}
			return nil
		})
	return corrected, err
}

func (s *service) reconcileActors(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var corrected int64
	err := pagination.EnumerateThrottled(
		ctx,
		s.ListActorsBuilder().Limit(batchSize).Enumerate,
		limiter,
		func(a *Actor) error {
			wasCorrected, err := s.recomputeActorLabelsTx(ctx, a.Id)
			if err != nil {
				return err
			}
			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", "actor", "id", a.Id)
			}
			return nil
		})
	return corrected, err
}

func (s *service) reconcileEncryptionKeys(ctx context.Context, batchSize int32, limiter *rate.Limiter) (int64, error) {
	var corrected int64
	err := pagination.EnumerateThrottled(
		ctx,
		s.ListEncryptionKeysBuilder().Limit(batchSize).Enumerate,
		limiter,
		func(ek EncryptionKey) error {
			wasCorrected, err := s.recomputeEncryptionKeyLabelsTx(ctx, ek.Id)
			if err != nil {
				return err
			}
			if wasCorrected {
				corrected++
				s.logger.Info("reconciled drifted carry-forward labels", "type", "encryption_key", "id", ek.Id)
			}
			return nil
		})
	return corrected, err
}
