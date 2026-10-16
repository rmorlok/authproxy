package database

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// copyToolMetadataForTest captures caller-owned maps independently so failed
// writes cannot pass unchanged-candidate assertions through map aliasing.
func copyToolMetadataForTest(tool *Tool) Tool {
	copy := *tool
	copy.Labels = tool.Labels.Copy()
	copy.Annotations = tool.Annotations.Copy()
	copy.Verbs = slices.Clone(tool.Verbs)
	return copy
}

// TestToolMetadataLifecycle persists only authored annotations and derives label
// identities from parents while detaching both successful input maps.
func TestToolMetadataLifecycle(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	_, err := db.UpdateNamespaceLabels(ctx, base.Namespace, map[string]string{"team": "platform"})
	require.NoError(t, err)
	_, err = db.UpdateNamespaceAnnotations(ctx, base.Namespace, map[string]string{"namespace-note": "private"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, base.Namespace))
	_, err = db.UpdateConnectionLabels(ctx, base.ConnectionId, map[string]string{"env": "prod"})
	require.NoError(t, err)
	_, err = db.UpdateConnectionAnnotations(ctx, base.ConnectionId, map[string]string{"connection-note": "private"})
	require.NoError(t, err)

	labels := Labels{
		"purpose":           "lookup",
		"apxy/cxn/-/id":     "cxn_forged",
		"apxy/tol/-/id":     "tol_forged",
		"apxy/ns/team":      "forged",
		"apxy/obsolete/key": "stale",
	}
	annotations := Annotations{"example.com/note": "line one\nline two"}
	tool := &Tool{
		Id:           apid.New(apid.PrefixTool),
		Namespace:    base.Namespace,
		Name:         "lookup",
		ConnectionId: base.ConnectionId,
		Description:  base.Description,
		Verbs:        base.Verbs,
		Labels:       labels,
		Annotations:  annotations,
	}
	revision, err := db.CreateStandaloneTool(ctx, tool, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, uint64(1), revision.Revision)
	require.Equal(t, "lookup", tool.Labels["purpose"])
	require.Equal(t, "platform", tool.Labels["apxy/ns/team"])
	require.Equal(t, "prod", tool.Labels["apxy/cxn/env"])
	require.Equal(t, base.ConnectionId.String(), tool.Labels["apxy/cxn/-/id"])
	require.Equal(t, tool.Id.String(), tool.Labels["apxy/tol/-/id"])
	require.Equal(t, "lookup", tool.Labels["apxy/tol/-/name"])
	require.Equal(t, tool.Namespace, tool.Labels["apxy/tol/-/ns"])
	require.NotContains(t, tool.Labels, "apxy/obsolete/key")
	require.Equal(t, annotations, tool.Annotations, "parent annotations are never inherited")
	require.Equal(t, "cxn_forged", labels["apxy/cxn/-/id"], "derivation does not rewrite the input map")
	labels["purpose"] = "mutated"
	annotations["example.com/note"] = "mutated"
	require.Equal(t, "lookup", tool.Labels["purpose"])
	require.Equal(t, "line one\nline two", tool.Annotations["example.com/note"])

	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.Labels, stored.Labels)
	require.Equal(t, tool.Annotations, stored.Annotations)
	stored.Labels["purpose"] = "changed"
	stored.Annotations["example.com/note"] = "new note"
	inputLabels, inputAnnotations := stored.Labels, stored.Annotations
	_, err = db.UpdateStandaloneTool(ctx, stored, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	inputLabels["purpose"] = "external mutation"
	inputAnnotations["example.com/note"] = "external mutation"
	require.Equal(t, "changed", stored.Labels["purpose"])
	require.Equal(t, "new note", stored.Annotations["example.com/note"])
	require.Equal(t, uint64(2), stored.ActiveRevision)

	// Full replacement with nil maps clears authored metadata but retains the
	// system projection needed to select the actual connection.
	stored.Labels = nil
	stored.Annotations = nil
	_, err = db.UpdateStandaloneTool(ctx, stored, 2, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	stored, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.NotContains(t, stored.Labels, "purpose")
	require.Empty(t, stored.Annotations)
	require.Equal(t, base.ConnectionId.String(), stored.Labels["apxy/cxn/-/id"])
}

// TestToolMetadataFailedWrites preserves candidate maps and persisted metadata
// when a stale revision or duplicate name aborts a replacement.
func TestToolMetadataFailedWrites(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	other := copyToolMetadataForTest(tool)
	other.Id = apid.New(apid.PrefixTool)
	other.Name = "occupied"
	_, err := db.CreateStandaloneTool(ctx, &other, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		revision uint64
		want     error
	}{
		{"stale", 2, ErrToolRevisionConflict},
		{"occupied", 1, ErrDuplicate},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := copyToolMetadataForTest(tool)
			candidate.Name = common.ResourceName(test.name)
			candidate.Labels = Labels{"new": "value", "apxy/cxn/-/id": "cxn_forged"}
			candidate.Annotations = Annotations{"new": "note"}
			before := copyToolMetadataForTest(&candidate)
			_, err := db.UpdateStandaloneTool(ctx, &candidate, test.revision, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.ErrorIs(t, err, test.want)
			require.Equal(t, before, candidate)
			stored, err := db.GetTool(ctx, tool.Id)
			require.NoError(t, err)
			require.Equal(t, tool.Labels, stored.Labels)
			require.Equal(t, tool.Annotations, stored.Annotations)
			require.Equal(t, uint64(1), stored.ActiveRevision)
			require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
		})
	}
}

// TestToolMetadataRetarget derives protected connection identity from columns,
// drops the old parent's labels, and supports selectors over the persisted map.
func TestToolMetadataRetarget(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	_, err := db.UpdateConnectionLabels(ctx, tool.ConnectionId, map[string]string{"old": "remove"})
	require.NoError(t, err)
	tool.Labels = Labels{"purpose": "lookup"}
	tool.Annotations = Annotations{"note": "preserved"}
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, "remove", tool.Labels["apxy/cxn/old"])

	connection := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)
	s := db.(*service)
	_, err = s.sq.Update(ConnectionsTable).
		Set("labels", Labels{
			"fresh":           "new",
			"apxy/cxn/-/id":   "cxn_forged",
			"apxy/cxn/-/name": "forged",
			"apxy/cxn/-/ns":   "root.forged",
			"apxy/ns/-/id":    "root.forged",
			"apxy/ns/-/name":  "forged",
			"apxy/ns/-/ns":    "root.forged",
			"apxy/tol/-/id":   "tol_forged",
		}).
		Where(sq.Eq{"id": connection.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	tool.ConnectionId = connection.Id
	tool.Name = "renamed"
	_, err = db.UpdateStandaloneTool(ctx, tool, 2, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NotContains(t, tool.Labels, "apxy/cxn/old")
	require.Equal(t, "new", tool.Labels["apxy/cxn/fresh"])
	require.Equal(t, connection.Id.String(), tool.Labels["apxy/cxn/-/id"])
	require.Equal(t, string(connection.Name), tool.Labels["apxy/cxn/-/name"])
	require.Equal(t, connection.Namespace, tool.Labels["apxy/cxn/-/ns"])
	require.Equal(t, tool.Namespace, tool.Labels["apxy/ns/-/id"])
	require.Equal(t, "owner", tool.Labels["apxy/ns/-/name"])
	require.Equal(t, tool.Namespace, tool.Labels["apxy/ns/-/ns"])
	require.Equal(t, tool.Id.String(), tool.Labels["apxy/tol/-/id"])
	require.Equal(t, "renamed", tool.Labels["apxy/tol/-/name"])
	require.Equal(t, Annotations{"note": "preserved"}, tool.Annotations)

	for _, test := range []struct {
		connection string
		want       int
	}{
		{connection.Id.String(), 1},
		{"cxn_forged", 0},
	} {
		selector, err := ParseLabelSelector("purpose=lookup,apxy/cxn/-/id=" + test.connection)
		require.NoError(t, err)
		query := selector.ApplyToSqlBuilderWithProvider(s.sq.Select("COUNT(*)").From(ToolsTable), "labels", s.cfg.GetProvider())
		var count int
		err = query.RunWith(s.db).QueryRowContext(ctx).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, test.want, count)
	}
}

// TestToolMetadataValidation applies shared metadata grammar and size bounds to
// models and writes without changing the caller or appending rejected history.
func TestToolMetadataValidation(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	for _, change := range []func(*Tool){
		func(v *Tool) { v.Labels = Labels{"invalid/key/name": "value"} },
		func(v *Tool) { v.Labels = Labels{"key": strings.Repeat("a", meta.LabelValueMaxLength+1)} },
		func(v *Tool) { v.Annotations = Annotations{"invalid/key/name": "value"} },
		func(v *Tool) { v.Annotations = Annotations{"key": strings.Repeat("a", meta.AnnotationsTotalMaxSize)} },
	} {
		invalid := copyToolMetadataForTest(tool)
		change(&invalid)
		before := copyToolMetadataForTest(&invalid)
		require.Error(t, invalid.Validate())
		_, err := db.UpdateStandaloneTool(ctx, &invalid, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.Error(t, err)
		require.Equal(t, before, invalid)
		invalid.Id = apid.New(apid.PrefixTool)
		invalid.Name = common.ResourceName(invalid.Id.String())
		before = copyToolMetadataForTest(&invalid)
		_, err = db.CreateStandaloneTool(ctx, &invalid, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.Error(t, err)
		require.Equal(t, before, invalid)
		require.Zero(t, countToolRevisionsForTest(t, ctx, db, invalid.Id))
	}
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// readToolRowForTest inspects retained physical rows without management liveness
// filters, allowing reconciliation tests to check tombstones and deleted parents.
func readToolRowForTest(t *testing.T, ctx context.Context, db DB, id apid.ID) *Tool {
	t.Helper()
	s := db.(*service)
	var tool Tool
	err := s.sq.Select(tool.cols()...).
		From(ToolsTable).
		Where(sq.Eq{"id": id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(tool.fields()...)
	require.NoError(t, err)
	return &tool
}

// TestToolMetadataPropagation walks namespace descendants and connector children
// without revising definitions or inheriting annotation payloads.
func TestToolMetadataPropagation(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	connection, err := db.GetConnection(ctx, tool.ConnectionId)
	require.NoError(t, err)
	require.NoError(t, db.UpsertConnectorGeneration(ctx, &ConnectorWithDefinition{
		Id:                  connection.ConnectorId,
		Namespace:           "root",
		Name:                "provider",
		Generation:          1,
		State:               ConnectorGenerationStatePrimary,
		Labels:              Labels{"service": "old"},
		Annotations:         Annotations{"provider-note": "private"},
		EncryptedDefinition: definition.EncryptedDefinition,
	}))
	tool.Labels = Labels{"local": "preserved"}
	tool.Annotations = Annotations{"note": "own"}
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	_, err = db.UpdateNamespaceLabels(ctx, "root", map[string]string{"team": "platform"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, "root"))
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, "platform", stored.Labels["apxy/ns/team"])
	require.Equal(t, "old", stored.Labels["apxy/cxr/service"])

	_, err = db.UpdateNamespaceLabels(ctx, "root", map[string]string{"region": "west"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, "root"))
	_, err = db.UpdateConnectorLabels(ctx, connection.ConnectorId, map[string]string{"service": "new"})
	require.NoError(t, err)
	require.NoError(t, db.RefreshConnectionsForConnector(ctx, connection.ConnectorId))
	stored, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.NotContains(t, stored.Labels, "apxy/ns/team")
	require.Equal(t, "west", stored.Labels["apxy/ns/region"])
	require.Equal(t, "new", stored.Labels["apxy/cxr/service"])
	require.Equal(t, "preserved", stored.Labels["local"])
	require.Equal(t, Annotations{"note": "own"}, stored.Annotations)
	require.Equal(t, uint64(2), stored.ActiveRevision)

	// A repeated connector refresh must visit Tool children even when their
	// connection's own projection already needs no correction.
	s := db.(*service)
	_, err = s.sq.Update(ToolsTable).
		Set("labels", Labels{"local": "preserved"}).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	require.NoError(t, db.RefreshConnectionsForConnector(ctx, connection.ConnectorId))
	stored, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, "new", stored.Labels["apxy/cxr/service"])
	require.Equal(t, connection.Id.String(), stored.Labels["apxy/cxn/-/id"])
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Revision)
	require.Equal(t, definition.EncryptedDefinition, current.EncryptedDefinition)
	require.Equal(t, definition.DefinitionDigest, current.DefinitionDigest)
	require.True(t, definition.CreatedAt.Equal(current.CreatedAt))
	require.True(t, definition.EncryptedAt.Equal(*current.EncryptedAt))
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestToolMetadataReconciliation repairs bounded pages and deleted-connection
// management projections, while skipping deleted Tools and namespaces.
func TestToolMetadataReconciliation(t *testing.T) {
	ctx, db, first, definition := standaloneToolForTest(t)
	first.Labels = Labels{"local": "preserved"}
	first.Annotations = Annotations{"note": "own"}
	_, err := db.UpdateStandaloneTool(ctx, first, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	tools := []*Tool{first}
	for i := range 2 {
		candidate := copyToolMetadataForTest(first)
		candidate.Id = apid.New(apid.PrefixTool)
		candidate.Name = common.ResourceName(fmt.Sprintf("lookup-%d", i))
		_, err := db.CreateStandaloneTool(ctx, &candidate, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.NoError(t, err)
		tools = append(tools, &candidate)
	}

	deadConnection := standaloneToolConnectionForTest(t, ctx, db, first.Namespace)
	retained := copyToolMetadataForTest(first)
	retained.Id = apid.New(apid.PrefixTool)
	retained.Name = "retained"
	retained.ConnectionId = deadConnection.Id
	_, err = db.CreateStandaloneTool(ctx, &retained, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteConnection(ctx, deadConnection.Id))
	tools = append(tools, &retained)

	deleted := copyToolMetadataForTest(first)
	deleted.Id = apid.New(apid.PrefixTool)
	deleted.Name = "deleted"
	_, err = db.CreateStandaloneTool(ctx, &deleted, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteStandaloneTool(ctx, deleted.Id, 1))
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.retired"}))
	retiredConnection := standaloneToolConnectionForTest(t, ctx, db, "root.retired")
	retired := copyToolMetadataForTest(first)
	retired.Id = apid.New(apid.PrefixTool)
	retired.Name = "retired"
	retired.Namespace = retiredConnection.Namespace
	retired.ConnectionId = retiredConnection.Id
	_, err = db.CreateStandaloneTool(ctx, &retired, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteNamespace(ctx, retired.Namespace))

	// Drain unrelated initial drift before measuring only these Tool repairs.
	_, err = db.ReconcileCarryForwardLabels(ctx, 1, nil)
	require.NoError(t, err)
	_, err = db.UpdateConnectionLabels(ctx, first.ConnectionId, map[string]string{"account": "updated"})
	require.NoError(t, err)
	s := db.(*service)
	drift := Labels{"local": "preserved", "apxy/cxn/-/id": "cxn_forged"}
	all := append(append([]*Tool{}, tools...), &deleted, &retired)
	for _, tool := range all {
		_, err := s.sq.Update(ToolsTable).
			Set("labels", drift).
			Where(sq.Eq{"id": tool.Id}).
			RunWith(s.db).
			ExecContext(ctx)
		require.NoError(t, err)
	}
	beforeDeleted := readToolRowForTest(t, ctx, db, deleted.Id)
	beforeRetired := readToolRowForTest(t, ctx, db, retired.Id)
	repairedAt := apctx.GetClock(ctx).Now().Add(time.Hour)
	fixed, err := db.ReconcileCarryForwardLabels(apctx.WithFixedClock(ctx, repairedAt), 1, nil)
	require.NoError(t, err)
	require.Equal(t, int64(len(tools)), fixed)
	for _, tool := range tools {
		stored, err := db.GetTool(ctx, tool.Id)
		require.NoError(t, err)
		require.Equal(t, tool.ConnectionId.String(), stored.Labels["apxy/cxn/-/id"])
		require.Equal(t, tool.Id.String(), stored.Labels["apxy/tol/-/id"])
		require.Equal(t, "preserved", stored.Labels["local"])
		if tool.ConnectionId == first.ConnectionId {
			require.Equal(t, "updated", stored.Labels["apxy/cxn/account"])
		}
		require.Equal(t, tool.Annotations, stored.Annotations)
		require.Equal(t, tool.ActiveRevision, stored.ActiveRevision)
		require.True(t, repairedAt.Equal(stored.UpdatedAt))
		revision, err := db.GetToolRevision(ctx, tool.Id, tool.ActiveRevision)
		require.NoError(t, err)
		require.Equal(t, definition.EncryptedDefinition, revision.EncryptedDefinition)
		require.Equal(t, definition.DefinitionDigest, revision.DefinitionDigest)
		require.True(t, definition.EncryptedAt.Equal(*revision.EncryptedAt))
		require.Equal(t, int(tool.ActiveRevision), countToolRevisionsForTest(t, ctx, db, tool.Id))
	}
	require.Equal(t, beforeDeleted, readToolRowForTest(t, ctx, db, deleted.Id))
	require.Equal(t, beforeRetired, readToolRowForTest(t, ctx, db, retired.Id))
	fixed, err = db.ReconcileCarryForwardLabels(apctx.WithFixedClock(ctx, repairedAt.Add(time.Hour)), 1, nil)
	require.NoError(t, err)
	require.Zero(t, fixed)
	for _, tool := range tools {
		stored, err := db.GetTool(ctx, tool.Id)
		require.NoError(t, err)
		require.True(t, repairedAt.Equal(stored.UpdatedAt), "a no-op repair must not rewrite timestamps")
	}
}

// TestToolMetadataConcurrentRepair preserves a committed rename, retarget, and
// authored metadata regardless of whether drift repair runs before or after it.
func TestToolMetadataConcurrentRepair(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	s := db.(*service)
	s.db.SetMaxOpenConns(2)
	connection := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)
	_, err := db.UpdateConnectionLabels(ctx, connection.Id, map[string]string{"fresh": "parent"})
	require.NoError(t, err)
	_, err = s.sq.Update(ToolsTable).
		Set("labels", Labels{"local": "before", "apxy/cxn/-/id": "cxn_drifted"}).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		ExecContext(ctx)
	require.NoError(t, err)
	candidate := copyToolMetadataForTest(tool)
	candidate.Name = "renamed"
	candidate.ConnectionId = connection.Id
	candidate.Labels = Labels{"local": "after"}
	candidate.Annotations = Annotations{"note": "after"}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := db.UpdateStandaloneTool(ctx, &candidate, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
		results <- err
	}()
	go func() {
		<-start
		_, err := s.recomputeToolLabelsTx(ctx, tool.Id)
		results <- err
	}()
	close(start)
	for range 2 {
		require.NoError(t, <-results)
	}
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, common.ResourceName("renamed"), stored.Name)
	require.Equal(t, connection.Id, stored.ConnectionId)
	require.Equal(t, "after", stored.Labels["local"])
	require.Equal(t, "parent", stored.Labels["apxy/cxn/fresh"])
	require.Equal(t, connection.Id.String(), stored.Labels["apxy/cxn/-/id"])
	require.Equal(t, "renamed", stored.Labels["apxy/tol/-/name"])
	require.Equal(t, Annotations{"note": "after"}, stored.Annotations)
	require.Equal(t, uint64(2), stored.ActiveRevision)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, connection.Id, current.ConnectionId)
	require.Equal(t, uint64(2), current.Revision)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestToolMetadataParentLiveness rejects dead destinations without partial
// metadata writes, while allowing recovery from a deleted former connection.
func TestToolMetadataParentLiveness(t *testing.T) {
	for _, parent := range []string{"connection", "namespace"} {
		t.Run(parent, func(t *testing.T) {
			ctx, db, tool, definition := standaloneToolForTest(t)
			destination := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)
			if parent == "connection" {
				require.NoError(t, db.DeleteConnection(ctx, destination.Id))
			} else {
				require.NoError(t, db.DeleteNamespace(ctx, tool.Namespace))
			}
			candidate := copyToolMetadataForTest(tool)
			candidate.ConnectionId = destination.Id
			candidate.Labels = Labels{"new": "label"}
			candidate.Annotations = Annotations{"new": "note"}
			before := copyToolMetadataForTest(&candidate)
			_, err := db.UpdateStandaloneTool(ctx, &candidate, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.ErrorIs(t, err, ErrNotFound)
			require.Equal(t, before, candidate)
			stored := readToolRowForTest(t, ctx, db, tool.Id)
			require.Equal(t, tool.Labels, stored.Labels)
			require.Equal(t, tool.Annotations, stored.Annotations)
			require.Equal(t, uint64(1), stored.ActiveRevision)
			require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))

			candidate.Id = apid.New(apid.PrefixTool)
			candidate.Name = "new-tool"
			before = copyToolMetadataForTest(&candidate)
			_, err = db.CreateStandaloneTool(ctx, &candidate, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.ErrorIs(t, err, ErrNotFound)
			require.Equal(t, before, candidate)
			require.Zero(t, countToolRevisionsForTest(t, ctx, db, candidate.Id))

			if parent == "connection" {
				require.NoError(t, db.DeleteConnection(ctx, tool.ConnectionId))
				live := standaloneToolConnectionForTest(t, ctx, db, tool.Namespace)
				tool.ConnectionId = live.Id
				tool.Labels = Labels{"recovered": "yes"}
				_, err := db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
				require.NoError(t, err)
				current, err := db.GetCurrentToolRevision(ctx, tool.Id)
				require.NoError(t, err)
				require.Equal(t, live.Id, current.ConnectionId)
				require.Equal(t, live.Id.String(), tool.Labels["apxy/cxn/-/id"])
				require.Equal(t, "yes", tool.Labels["recovered"])
			}
		})
	}
}

// TestToolMetadataMigrationReversibility retains populated identities and
// history through downgrade, then repairs NULL metadata after upgrading again.
func TestToolMetadataMigrationReversibility(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	tool.Labels = Labels{"local": "before"}
	tool.Annotations = Annotations{"note": "before"}
	_, err := db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	s := db.(*service)
	migrateDatabaseToVersion(t, s, 20)
	var connectionID apid.ID
	var activeRevision uint64
	var name common.ResourceName
	err = s.sq.Select("connection_id", "active_revision", "name").
		From(ToolsTable).
		Where(sq.Eq{"id": tool.Id}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(&connectionID, &activeRevision, &name)
	require.NoError(t, err)
	require.Equal(t, tool.ConnectionId, connectionID)
	require.Equal(t, tool.ActiveRevision, activeRevision)
	require.Equal(t, tool.Name, name)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
	var revision ToolRevision
	err = s.sq.Select(revision.cols()...).
		From(ToolRevisionsTable).
		Where(sq.Eq{"tool_id": tool.Id, "revision": 2}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(revision.fields()...)
	require.NoError(t, err)
	require.Equal(t, definition.EncryptedDefinition, revision.EncryptedDefinition)
	require.Equal(t, definition.DefinitionDigest, revision.DefinitionDigest)

	migrateDatabaseToVersion(t, s, 22)
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Nil(t, stored.Labels)
	require.Nil(t, stored.Annotations)
	_, err = db.ReconcileCarryForwardLabels(ctx, 1, nil)
	require.NoError(t, err)
	stored, err = db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.Id.String(), stored.Labels["apxy/tol/-/id"])
	require.Equal(t, tool.ConnectionId.String(), stored.Labels["apxy/cxn/-/id"])
	require.NotContains(t, stored.Labels, "local", "downgrade removes the metadata columns")
	require.Empty(t, stored.Annotations)
	require.Equal(t, uint64(2), stored.ActiveRevision)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, revision.EncryptedDefinition, current.EncryptedDefinition)
	require.Equal(t, revision.DefinitionDigest, current.DefinitionDigest)
}

// TestToolMetadataRepairLockOrder ensures PostgreSQL label repair never waits
// for a connection lock held by an authoring transaction that will next lock
// the Tool. A second tuple write would otherwise trigger foreign-key rechecks.
func TestToolMetadataRepairLockOrder(t *testing.T) {
	ctx, db, tool, _ := standaloneToolForTest(t)
	s := db.(*service)
	if s.cfg.GetProvider() != config.DatabaseProviderPostgres {
		t.Skip("SQLite serializes writers; this checks PostgreSQL foreign-key lock ordering")
	}
	// The shared fixture may limit this isolated database to one connection.
	// Two real transactions are required to exercise competing row locks.
	s.db.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	parentTx, err := s.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer parentTx.Rollback()
	_, err = s.sq.Update(ConnectionsTable).
		Set("id", tool.ConnectionId).
		Where(sq.Eq{"id": tool.ConnectionId}).
		RunWith(parentTx).
		ExecContext(ctx)
	require.NoError(t, err)

	repairTx, err := s.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer repairTx.Rollback()
	_, err = repairTx.ExecContext(ctx, "SET LOCAL lock_timeout = '250ms'")
	require.NoError(t, err)
	require.NoError(t, s.lockToolForWrite(ctx, repairTx, tool.Id))
	repaired := tool.Labels.Copy()
	repaired["repaired"] = "yes"
	changed, err := s.writeRecomputedLabels(ctx, repairTx, ToolsTable, sq.Eq{"id": tool.Id}, tool.Labels, repaired)
	require.NoError(t, err, "repair must finish while the parent lock is still held")
	require.True(t, changed)
	require.NoError(t, repairTx.Commit())
	require.NoError(t, parentTx.Rollback())
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, repaired, stored.Labels)
}

// TestToolMetadataNamespaceOverridesStaleConnection derives namespace labels
// from the current namespace even when connection projection repair cannot run.
func TestToolMetadataNamespaceOverridesStaleConnection(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("connectionDeleted=%v", deleted), func(t *testing.T) {
			ctx, db, tool, definition := standaloneToolForTest(t)
			tool.Labels = Labels{"local": "preserved"}
			tool.Annotations = Annotations{"note": "preserved"}
			_, err := db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.NoError(t, err)
			_, err = db.UpdateNamespaceLabels(ctx, tool.Namespace, map[string]string{"changed": "old", "removed": "old"})
			require.NoError(t, err)
			require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, tool.Namespace))
			if deleted {
				require.NoError(t, db.DeleteConnection(ctx, tool.ConnectionId))
			}
			_, err = db.UpdateNamespaceLabels(ctx, tool.Namespace, map[string]string{"changed": "new"})
			require.NoError(t, err)
			s := db.(*service)
			if deleted {
				require.NoError(t, db.RefreshNamespaceLabelsCarryForward(ctx, tool.Namespace))
			} else {
				// Leave the live connection unrefreshed to exercise the same
				// precedence when propagation has not reached that row yet.
				_, err = s.recomputeToolLabelsTx(ctx, tool.Id)
				require.NoError(t, err)
			}

			stored, err := db.GetTool(ctx, tool.Id)
			require.NoError(t, err)
			require.Equal(t, "new", stored.Labels["apxy/ns/changed"])
			require.NotContains(t, stored.Labels, "apxy/ns/removed")
			require.Equal(t, "preserved", stored.Labels["local"])
			require.Equal(t, tool.Annotations, stored.Annotations)
			require.Equal(t, uint64(2), stored.ActiveRevision)
			revision, err := db.GetToolRevision(ctx, tool.Id, 2)
			require.NoError(t, err)
			require.Equal(t, definition.EncryptedDefinition, revision.EncryptedDefinition)
			require.Equal(t, definition.DefinitionDigest, revision.DefinitionDigest)
			require.True(t, definition.EncryptedAt.Equal(*revision.EncryptedAt))
			require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))

			var connectionLabels Labels
			var deletedAt *time.Time
			err = s.sq.Select("labels", "deleted_at").
				From(ConnectionsTable).
				Where(sq.Eq{"id": tool.ConnectionId}).
				RunWith(s.db).
				QueryRowContext(ctx).
				Scan(&connectionLabels, &deletedAt)
			require.NoError(t, err)
			require.Equal(t, "old", connectionLabels["apxy/ns/changed"])
			require.Equal(t, "old", connectionLabels["apxy/ns/removed"])
			require.Equal(t, deleted, deletedAt != nil)
		})
	}
}
