package database

import (
	"context"
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// createToolCandidateForTest stores an independently named Tool with the same
// opaque definition as the fixture, optionally under another connection.
func createToolCandidateForTest(
	t *testing.T,
	ctx context.Context,
	db DB,
	base *Tool,
	definition *ToolRevision,
	connection *Connection,
	verbs []string,
	labels Labels,
) *Tool {
	t.Helper()
	tool := &Tool{
		Id:           apid.New(apid.PrefixTool),
		Namespace:    base.Namespace,
		ConnectionId: base.ConnectionId,
		Description:  base.Description,
		Verbs:        verbs,
		Labels:       labels,
	}
	if connection != nil {
		tool.Namespace, tool.ConnectionId = connection.Namespace, connection.Id
	}
	_, err := db.CreateStandaloneTool(ctx, tool, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	return tool
}

// toolCandidateIDsForTest extracts result order without coupling assertions to
// unrelated fields in the candidate projection.
func toolCandidateIDsForTest(page ToolCandidatePage) []apid.ID {
	ids := make([]apid.ID, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.Id)
	}
	return ids
}

// TestToolCandidatesPagination applies every filter before page boundaries and
// preserves an ID cursor when previously returned rows are renamed or deleted.
func TestToolCandidatesPagination(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.outside"}))
	other := standaloneToolConnectionForTest(t, ctx, db, "root.outside")
	var candidates []*Tool
	// Numeric suffixes have the same ordering under both providers' collations.
	// Interleave each kind of nonmatch before valid rows to expose filters that
	// accidentally run after the query's limit.
	for index := range 8 {
		tool := &Tool{
			Id:           apid.ID(fmt.Sprintf("tol_%016d", index)),
			Namespace:    base.Namespace,
			ConnectionId: base.ConnectionId,
			Description:  base.Description,
			Verbs:        []string{"read"},
			Labels:       Labels{"eligible": "yes"},
		}
		if index == 0 || index == 4 || index == 6 {
			tool.Labels["eligible"] = "no"
		} else if index == 1 {
			tool.Namespace, tool.ConnectionId = other.Namespace, other.Id
		} else if index == 2 {
			tool.Verbs = []string{"write"}
		}
		_, err := db.CreateStandaloneTool(ctx, tool, definition.EncryptedDefinition, definition.DefinitionDigest)
		require.NoError(t, err)
		candidates = append(candidates, tool)
	}
	query := ToolCandidateQuery{NamespaceMatchers: []string{base.Namespace}, LabelSelector: "eligible=yes", AnyVerbs: []string{"read"}, Limit: 2}
	first, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[3].Id, candidates[5].Id}, toolCandidateIDsForTest(first))
	require.Equal(t, candidates[5].Id, first.NextAfterID)
	allQuery := query
	allQuery.Limit = MaxToolCandidateLimit
	all, err := db.ListToolCandidates(ctx, allQuery)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[3].Id, candidates[5].Id, candidates[7].Id}, toolCandidateIDsForTest(all))
	require.True(t, all.NextAfterID.IsNil())
	query.AfterID = candidates[3].Id
	exact, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[5].Id, candidates[7].Id}, toolCandidateIDsForTest(exact))
	require.True(t, exact.NextAfterID.IsNil(), "a full final page has no continuation")

	candidates[3].Name = "renamed-before-cursor"
	_, err = db.UpdateStandaloneTool(ctx, candidates[3], 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteStandaloneTool(ctx, candidates[5].Id, 1))
	query.AfterID = first.NextAfterID
	last, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[7].Id}, toolCandidateIDsForTest(last))
	require.True(t, last.NextAfterID.IsNil())
	require.Equal(t, toolCandidateIDsForTest(all), append(toolCandidateIDsForTest(first), toolCandidateIDsForTest(last)...))
	query.AfterID = candidates[7].Id
	empty, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.True(t, empty.NextAfterID.IsNil())
}

// TestToolCandidatesNamespaceAndLabels combines exact and descendant scopes
// without prefix leakage, then selects an actual inherited connection identity.
func TestToolCandidatesNamespaceAndLabels(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	byNamespace := map[string]apid.ID{base.Namespace: base.Id}
	for _, path := range []string{"root.owner.child", "root.ownerish", "root.other", "root.user_1", "root.user_1.child", "root.userA1.child", "root.USER_1.child", "root.user_10"} {
		require.NoError(t, db.EnsureNamespaceByPath(ctx, path))
		connection := standaloneToolConnectionForTest(t, ctx, db, path)
		tool := createToolCandidateForTest(t, ctx, db, base, definition, connection, base.Verbs, nil)
		byNamespace[path] = tool.Id
	}
	for _, test := range []struct {
		matchers []string
		want     []apid.ID
	}{
		{nil, nil}, {[]string{}, nil},
		{[]string{"root.owner"}, []apid.ID{base.Id}},
		{[]string{"root.owner.**"}, []apid.ID{base.Id, byNamespace["root.owner.child"]}},
		{[]string{"root.user_1.**"}, []apid.ID{byNamespace["root.user_1"], byNamespace["root.user_1.child"]}},
		{[]string{"root.owner.**", "root.owner", "root.other"}, []apid.ID{base.Id, byNamespace["root.owner.child"], byNamespace["root.other"]}},
	} {
		page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: test.matchers, Limit: 10})
		require.NoError(t, err)
		require.ElementsMatch(t, test.want, toolCandidateIDsForTest(page))
		require.True(t, page.NextAfterID.IsNil())
	}
	for _, selector := range []string{
		"apxy/cxn/-/id=" + base.ConnectionId.String(),
		"apxy/cxn/-/id==" + base.ConnectionId.String() + ",apxy/tol/-/id,!missing",
	} {
		page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, LabelSelector: selector, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{base.Id}, toolCandidateIDsForTest(page))
		require.True(t, page.NextAfterID.IsNil())
	}
}

// TestToolCandidatesExactVerbs treats permission aliases as literal strings,
// returns a Tool once for overlapping aliases, and reads replacement aliases.
func TestToolCandidatesExactVerbs(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	verbs := []string{"Read", "read:item", "a'b", "50%_done", "read details", "*", `quote"slash\`}
	tool := createToolCandidateForTest(t, ctx, db, base, definition, nil, verbs, Labels{"eligible": "yes"})
	decoy := createToolCandidateForTest(t, ctx, db, base, definition, nil, []string{"read", "readXitem", "ab", "50XXdone", "read  details", "anything", "quoteslash"}, Labels{"eligible": "yes"})
	query := ToolCandidateQuery{NamespaceMatchers: []string{base.Namespace}, LabelSelector: "eligible=yes", Limit: 10}
	for _, verb := range verbs {
		query.AnyVerbs = []string{verb}
		page, err := db.ListToolCandidates(ctx, query)
		require.NoError(t, err, verb)
		require.Equal(t, []apid.ID{tool.Id}, toolCandidateIDsForTest(page), verb)
	}
	query.AnyVerbs = []string{"Read", "Read", "read:item"}
	page, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{tool.Id}, toolCandidateIDsForTest(page))
	query.AnyVerbs = nil
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.ElementsMatch(t, []apid.ID{tool.Id, decoy.Id}, toolCandidateIDsForTest(page))
	query.AnyVerbs = []string{}
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	tool.Verbs, tool.Description = []string{"replacement"}, "Updated operation"
	_, err = db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	query.AnyVerbs = verbs
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	query.AnyVerbs = tool.Verbs
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, uint64(2), page.Items[0].ActiveRevision)
	require.Equal(t, tool.Description, page.Items[0].Description)
}

// TestToolCandidatesLiveness excludes retained or incomplete storage rows while
// leaving connection readiness decisions to the caller.
func TestToolCandidatesLiveness(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	s := db.(*service)
	for index, change := range []func(*Tool, *Connection) sq.UpdateBuilder{
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolsTable).Set("deleted_at", apctx.GetClock(ctx).Now()).Where(sq.Eq{"id": v.Id})
		},
		func(_ *Tool, c *Connection) sq.UpdateBuilder {
			return s.sq.Update(ConnectionsTable).Set("deleted_at", apctx.GetClock(ctx).Now()).Where(sq.Eq{"id": c.Id})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(NamespacesTable).Set("deleted_at", apctx.GetClock(ctx).Now()).Where(sq.Eq{"path": v.Namespace})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolRevisionsTable).Set("deleted_at", apctx.GetClock(ctx).Now()).Where(sq.Eq{"tool_id": v.Id})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolRevisionsTable).Set("connection_id", base.ConnectionId).Where(sq.Eq{"tool_id": v.Id})
		},
		func(_ *Tool, c *Connection) sq.UpdateBuilder {
			return s.sq.Update(ConnectionsTable).Set("namespace", "root").Where(sq.Eq{"id": c.Id})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolsTable).Set("description", "").Where(sq.Eq{"id": v.Id})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolsTable).Set("verbs", nil).Where(sq.Eq{"id": v.Id})
		},
		func(v *Tool, _ *Connection) sq.UpdateBuilder {
			return s.sq.Update(ToolsTable).Set("verbs", "[]").Where(sq.Eq{"id": v.Id})
		},
	} {
		path := fmt.Sprintf("root.case-%d", index)
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path}))
		connection := standaloneToolConnectionForTest(t, ctx, db, path)
		tool := createToolCandidateForTest(t, ctx, db, base, definition, connection, base.Verbs, nil)
		_, err := change(tool, connection).RunWith(s.db).ExecContext(ctx)
		require.NoError(t, err)
	}
	for _, state := range []ConnectionState{ConnectionStateSetup, ConnectionStateDisabled, ConnectionStateDisconnecting, ConnectionStateDisconnected} {
		require.NoError(t, db.SetConnectionState(ctx, base.ConnectionId, state))
		page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{base.Id}, toolCandidateIDsForTest(page), state)
		require.True(t, page.NextAfterID.IsNil(), state)
	}
}

// TestToolCandidatesCurrentConnectionMetadata returns separate current parent
// labels and names, and caller mutations cannot change stored or later results.
func TestToolCandidatesCurrentConnectionMetadata(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	tool.Name = "lookup"
	tool.Labels = Labels{"purpose": "lookup"}
	_, err := db.UpdateStandaloneTool(ctx, tool, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	connection, err := db.UpdateConnectionName(ctx, tool.ConnectionId, common.ResourceName("renamed-parent"))
	require.NoError(t, err)
	connection, err = db.UpdateConnectionLabels(ctx, tool.ConnectionId, map[string]string{"team": "current"})
	require.NoError(t, err)
	// Discovery does not hydrate encrypted source. Make it unreadable by the
	// encrypted-field scanner to catch accidental full revision hydration.
	s := db.(*service)
	_, err = s.sq.Update(ToolRevisionsTable).Set("encrypted_definition", `"not an encrypted field"`).
		Where(sq.Eq{"tool_id": tool.Id}).RunWith(s.db).ExecContext(ctx)
	require.NoError(t, err)
	query := ToolCandidateQuery{NamespaceMatchers: []string{tool.Namespace}, Limit: 10}
	page, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	require.Equal(t, tool.Id, item.Id)
	require.Equal(t, tool.Namespace, item.Namespace)
	require.Equal(t, tool.Name, item.Name)
	require.Equal(t, connection.Id, item.ConnectionId)
	require.Equal(t, connection.Name, item.ConnectionName)
	require.Equal(t, tool.Labels, item.Labels)
	require.Equal(t, connection.Labels, item.ConnectionLabels)
	item.Labels["purpose"], item.ConnectionLabels["team"], item.Verbs[0] = "changed", "changed", "changed"
	again, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, tool.Labels, again.Items[0].Labels)
	require.Equal(t, connection.Labels, again.Items[0].ConnectionLabels)
	require.Equal(t, tool.Verbs, again.Items[0].Verbs)
}

// TestToolCandidatesValidation validates inputs even when an empty namespace or
// alias scope would otherwise short-circuit the query, and honors cancellation.
func TestToolCandidatesValidation(t *testing.T) {
	ctx, db, _, _ := standaloneToolForTest(t)
	for _, test := range []struct {
		name string
		edit func(*ToolCandidateQuery)
	}{
		{"zero limit", func(q *ToolCandidateQuery) { q.Limit = 0 }},
		{"negative limit", func(q *ToolCandidateQuery) { q.Limit = -1 }},
		{"large limit", func(q *ToolCandidateQuery) { q.Limit = MaxToolCandidateLimit + 1 }},
		{"matcher", func(q *ToolCandidateQuery) { q.NamespaceMatchers = []string{"root.**", "invalid**"} }},
		{"cursor type", func(q *ToolCandidateQuery) { q.AfterID = apid.New(apid.PrefixConnection) }},
		{"cursor syntax", func(q *ToolCandidateQuery) { q.AfterID = apid.ID("invalid") }},
		{"selector", func(q *ToolCandidateQuery) { q.LabelSelector = "key in (" }},
		{"blank verb", func(q *ToolCandidateQuery) { q.AnyVerbs = []string{"read", " \t"} }},
		{"padded verb", func(q *ToolCandidateQuery) { q.AnyVerbs = []string{"read "} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := ToolCandidateQuery{Limit: 1}
			test.edit(&query)
			_, err := db.ListToolCandidates(ctx, query)
			require.Error(t, err)
			query.NamespaceMatchers = append(query.NamespaceMatchers, "root.**")
			_, err = db.ListToolCandidates(ctx, query)
			require.Error(t, err)
			if query.AnyVerbs == nil {
				query.AnyVerbs = []string{}
				_, err = db.ListToolCandidates(ctx, query)
				require.Error(t, err)
			}
		})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := db.ListToolCandidates(canceled, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, Limit: 1})
	require.ErrorIs(t, err, context.Canceled)
}
