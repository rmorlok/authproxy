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
)

// recompute helpers re-derive the apxy/ portion of a child's labels column
// from the current state of its parents, preserving the user-supplied
// portion. They are invoked when a parent's labels (or, in principle, a
// resource's parent pointer) change so the materialized carry-forward
// stays in sync.

// recomputeConnectionLabels re-derives a connection's full labels from its
// current connector version + namespace + own user labels and persists the
// result.
func (s *service) recomputeConnectionLabels(ctx context.Context, tx *sql.Tx, id apid.ID) error {
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
			return ErrNotFound
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

	return s.writeRecomputedLabels(ctx, tx, ConnectionsTable, sq.Eq{"id": id, "deleted_at": nil}, newLabels)
}

// recomputeActorLabels re-derives an actor's full labels from its namespace
// and own user labels.
func (s *service) recomputeActorLabels(ctx context.Context, tx *sql.Tx, id apid.ID) error {
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
			return ErrNotFound
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

	return s.writeRecomputedLabels(ctx, tx, ActorTable, sq.Eq{"id": id, "deleted_at": nil}, newLabels)
}

// recomputeEncryptionKeyLabels re-derives an encryption key's full labels
// from its namespace and own user labels.
func (s *service) recomputeEncryptionKeyLabels(ctx context.Context, tx *sql.Tx, id apid.ID) error {
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
			return ErrNotFound
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

	return s.writeRecomputedLabels(ctx, tx, EncryptionKeysTable, sq.Eq{"id": id, "deleted_at": nil}, newLabels)
}

// recomputeConnectorVersionLabels re-derives a connector version's full
// labels from its namespace and own user labels.
func (s *service) recomputeConnectorVersionLabels(ctx context.Context, tx *sql.Tx, id apid.ID, version uint64) error {
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
			return ErrNotFound
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

	return s.writeRecomputedLabels(ctx, tx, ConnectorVersionsTable, sq.Eq{
		"id":         id,
		"version":    version,
		"deleted_at": nil,
	}, newLabels)
}

// recomputeNamespaceLabels re-derives a namespace's full labels from its
// immediate parent namespace and its own user labels.
func (s *service) recomputeNamespaceLabels(ctx context.Context, tx *sql.Tx, path string) error {
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
			return ErrNotFound
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

	return s.writeRecomputedLabels(ctx, tx, NamespacesTable, sq.Eq{"path": path, "deleted_at": nil}, newLabels)
}

// writeRecomputedLabels persists a recomputed labels map to a row. Updates
// the updated_at timestamp.
func (s *service) writeRecomputedLabels(ctx context.Context, tx *sql.Tx, table string, where sq.Eq, labels Labels) error {
	_, err := s.sq.
		Update(table).
		Set("labels", labels).
		Set("updated_at", apctx.GetClock(ctx).Now()).
		Where(where).
		RunWith(tx).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to write recomputed labels in %s: %w", table, err)
	}
	return nil
}

// propagateNamespaceLabelsChange refreshes the apxy/ portion of every
// resource that materializes labels from this namespace, then recurses into
// each direct child namespace so the chain stays consistent.
//
// Call this after a namespace's own user labels have been updated. The
// namespace's own row is expected to already reflect the new user labels;
// this function does not touch it.
func (s *service) propagateNamespaceLabelsChange(ctx context.Context, tx *sql.Tx, nsPath string) error {
	if err := s.refreshConnectionsInNamespace(ctx, tx, nsPath); err != nil {
		return err
	}
	if err := s.refreshActorsInNamespace(ctx, tx, nsPath); err != nil {
		return err
	}
	if err := s.refreshEncryptionKeysInNamespace(ctx, tx, nsPath); err != nil {
		return err
	}
	if err := s.refreshConnectorVersionsInNamespace(ctx, tx, nsPath); err != nil {
		return err
	}

	// Recurse into direct child namespaces. Each child re-derives its own
	// apxy/ns/* from the parent's just-updated labels, then propagates
	// onward to its own descendants.
	childPaths, err := s.directChildNamespacePaths(ctx, tx, nsPath)
	if err != nil {
		return err
	}
	for _, childPath := range childPaths {
		if err := s.recomputeNamespaceLabels(ctx, tx, childPath); err != nil {
			return err
		}
		if err := s.propagateNamespaceLabelsChange(ctx, tx, childPath); err != nil {
			return err
		}
	}
	return nil
}

// directChildNamespacePaths returns the paths of all immediate (depth+1)
// non-deleted child namespaces of nsPath.
func (s *service) directChildNamespacePaths(ctx context.Context, tx *sql.Tx, nsPath string) ([]string, error) {
	depth := DepthOfNamespacePath(nsPath) + 1
	rows, err := s.sq.
		Select("path").
		From(NamespacesTable).
		Where(sq.And{
			sq.Like{"path": nsPath + aschema.NamespacePathSeparator + "%"},
			sq.Eq{"depth": depth, "deleted_at": nil},
		}).
		RunWith(tx).
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

// refreshConnectionsInNamespace recomputes labels for every non-deleted
// connection whose own namespace is exactly nsPath.
func (s *service) refreshConnectionsInNamespace(ctx context.Context, tx *sql.Tx, nsPath string) error {
	ids, err := s.scanIdsByNamespace(tx, ConnectionsTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.recomputeConnectionLabels(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshActorsInNamespace recomputes labels for every non-deleted actor in
// nsPath.
func (s *service) refreshActorsInNamespace(ctx context.Context, tx *sql.Tx, nsPath string) error {
	ids, err := s.scanIdsByNamespace(tx, ActorTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.recomputeActorLabels(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshEncryptionKeysInNamespace recomputes labels for every non-deleted
// encryption key in nsPath.
func (s *service) refreshEncryptionKeysInNamespace(ctx context.Context, tx *sql.Tx, nsPath string) error {
	ids, err := s.scanIdsByNamespace(tx, EncryptionKeysTable, nsPath)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.recomputeEncryptionKeyLabels(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// refreshConnectorVersionsInNamespace recomputes labels for every
// non-deleted (id, version) connector version whose namespace is nsPath.
func (s *service) refreshConnectorVersionsInNamespace(ctx context.Context, tx *sql.Tx, nsPath string) error {
	rows, err := s.sq.
		Select("id", "version").
		From(ConnectorVersionsTable).
		Where(sq.Eq{"namespace": nsPath, "deleted_at": nil}).
		RunWith(tx).
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
		if err := s.recomputeConnectorVersionLabels(ctx, tx, ref.id, ref.version); err != nil {
			return err
		}
	}
	return nil
}

// scanIdsByNamespace returns the ids of every non-deleted row whose
// namespace column equals nsPath.
func (s *service) scanIdsByNamespace(tx *sql.Tx, table, nsPath string) ([]apid.ID, error) {
	rows, err := s.sq.
		Select("id").
		From(table).
		Where(sq.Eq{"namespace": nsPath, "deleted_at": nil}).
		RunWith(tx).
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

// refreshConnectionsForConnectorVersion recomputes labels for every
// non-deleted connection that points at the given connector (id, version).
// Used after a connector version's user labels change so connections see
// the new apxy/cxr/<k> immediately. In practice this only fires for draft
// connector versions — primary and active versions are immutable.
func (s *service) refreshConnectionsForConnectorVersion(ctx context.Context, tx *sql.Tx, id apid.ID, version uint64) error {
	rows, err := s.sq.
		Select("id").
		From(ConnectionsTable).
		Where(sq.Eq{
			"connector_id":      id,
			"connector_version": version,
			"deleted_at":        nil,
		}).
		RunWith(tx).
		Query()
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []apid.ID
	for rows.Next() {
		var connID apid.ID
		if err := rows.Scan(&connID); err != nil {
			return err
		}
		ids = append(ids, connID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, connID := range ids {
		if err := s.recomputeConnectionLabels(ctx, tx, connID); err != nil {
			return err
		}
	}
	return nil
}
