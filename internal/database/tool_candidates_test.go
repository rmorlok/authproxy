package database

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
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
// preserves an opaque continuation when previously returned rows change.
func TestToolCandidatesPagination(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: "root.outside"}))
	other := standaloneToolConnectionForTest(t, ctx, db, "root.outside")
	var candidates []*Tool
	// Numeric suffixes have the same ordering under both providers' collations.
	// Interleave each kind of nonmatch before valid rows to expose filters that
	// accidentally run after the query's limit.
	for index := range 12 {
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
		} else if index == 1 || index == 8 {
			tool.Namespace, tool.ConnectionId = other.Namespace, other.Id
		} else if index == 2 || index == 10 {
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
	require.NotEmpty(t, first.Cursor)
	second, err := db.ListToolCandidatesFromCursor(ctx, first.Cursor)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[7].Id, candidates[9].Id}, toolCandidateIDsForTest(second))
	require.NotEmpty(t, second.Cursor)
	last, err := db.ListToolCandidatesFromCursor(ctx, second.Cursor)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[11].Id}, toolCandidateIDsForTest(last))
	require.Empty(t, last.Cursor)
	allQuery := query
	allQuery.Limit = MaxToolCandidateLimit
	all, err := db.ListToolCandidates(ctx, allQuery)
	require.NoError(t, err)
	traversed := append(toolCandidateIDsForTest(first), toolCandidateIDsForTest(second)...)
	traversed = append(traversed, toolCandidateIDsForTest(last)...)
	require.Equal(t, toolCandidateIDsForTest(all), traversed)
	require.Empty(t, all.Cursor)

	candidates[3].Name = "renamed-before-cursor"
	_, err = db.UpdateStandaloneTool(ctx, candidates[3], 1, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteStandaloneTool(ctx, candidates[5].Id, 1))
	require.NoError(t, db.DeleteStandaloneTool(ctx, candidates[11].Id, 1))
	exact, err := db.ListToolCandidatesFromCursor(ctx, first.Cursor)
	require.NoError(t, err)
	require.Equal(t, []apid.ID{candidates[7].Id, candidates[9].Id}, toolCandidateIDsForTest(exact))
	require.Empty(t, exact.Cursor, "a full final page has no continuation")
	empty, err := db.ListToolCandidatesFromCursor(ctx, second.Cursor)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Empty(t, empty.Cursor)
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
		require.Empty(t, page.Cursor)
	}
	for _, selector := range []string{
		"apxy/cxn/-/id=" + base.ConnectionId.String(),
		"apxy/cxn/-/id==" + base.ConnectionId.String() + ",apxy/tol/-/id,!missing",
	} {
		page, err := db.ListToolCandidates(ctx, ToolCandidateQuery{NamespaceMatchers: []string{"root.**"}, LabelSelector: selector, Limit: 1})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{base.Id}, toolCandidateIDsForTest(page))
		require.Empty(t, page.Cursor)
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
	query.Limit = 1
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.NotEmpty(t, page.Cursor)
	next, err := db.ListToolCandidatesFromCursor(ctx, page.Cursor)
	require.NoError(t, err)
	require.Empty(t, next.Cursor)
	require.ElementsMatch(t, []apid.ID{tool.Id, decoy.Id}, append(toolCandidateIDsForTest(page), toolCandidateIDsForTest(next)...))
	query.Limit = 10
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
		require.Empty(t, page.Cursor, state)
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

// TestToolCandidatesCursorErrors rejects damaged or invalid continuation state
// and does not expose partial pages when their continuation cannot be encrypted.
func TestToolCandidatesCursorErrors(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	createToolCandidateForTest(t, ctx, db, base, definition, nil, base.Verbs, nil)
	query := ToolCandidateQuery{NamespaceMatchers: []string{base.Namespace}, Limit: 1}
	first, err := db.ListToolCandidates(ctx, query)
	require.NoError(t, err)
	require.NotEmpty(t, first.Cursor)

	// Alter ciphertext without breaking its outer encoding so authentication,
	// rather than only inline-string parsing, must reject the token.
	tampered, err := encfield.ParseInlineString(first.Cursor)
	require.NoError(t, err)
	ciphertext, err := base64.StdEncoding.DecodeString(tampered.Data)
	require.NoError(t, err)
	ciphertext[len(ciphertext)-1] ^= 1
	tampered.Data = base64.StdEncoding.EncodeToString(ciphertext)
	for _, cursor := range []string{"", "invalid", base.Id.String(), tampered.ToInlineString()} {
		page, err := db.ListToolCandidatesFromCursor(ctx, cursor)
		require.Error(t, err)
		require.Empty(t, page.Items)
		require.Empty(t, page.Cursor)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = db.ListToolCandidatesFromCursor(canceled, first.Cursor)
	require.ErrorIs(t, err, context.Canceled)

	// Restored payloads still require validation even though the token passed
	// decryption, for example after a producer bug or future format change.
	s := db.(*service)
	payload, err := pagination.ParseCursor[toolCandidateCursor](ctx, s.cursorEncryptor, first.Cursor)
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		edit func(*toolCandidateCursor)
	}{
		{"missing position", func(c *toolCandidateCursor) { c.AfterID = apid.Nil }},
		{"wrong position type", func(c *toolCandidateCursor) { c.AfterID = base.ConnectionId }},
		{"invalid position", func(c *toolCandidateCursor) { c.AfterID = apid.ID("invalid") }},
		{"zero limit", func(c *toolCandidateCursor) { c.Limit = 0 }},
		{"large limit", func(c *toolCandidateCursor) { c.Limit = MaxToolCandidateLimit + 1 }},
		{"matcher", func(c *toolCandidateCursor) { c.NamespaceMatchers = []string{"invalid**"} }},
		{"selector", func(c *toolCandidateCursor) { c.LabelSelector = "key in (" }},
		{"blank verb", func(c *toolCandidateCursor) { c.AnyVerbs = []string{" "} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := *payload
			test.edit(&invalid)
			cursor, err := pagination.MakeCursor(ctx, s.cursorEncryptor, invalid)
			require.NoError(t, err)
			page, err := db.ListToolCandidatesFromCursor(ctx, cursor)
			require.Error(t, err)
			require.Empty(t, page.Items)
			require.Empty(t, page.Cursor)
		})
	}
	// Empty aliases must survive encoding as an empty set, never become the nil
	// value that would omit the alias restriction entirely.
	payload.AnyVerbs = []string{}
	cursor, err := pagination.MakeCursor(ctx, s.cursorEncryptor, payload)
	require.NoError(t, err)
	page, err := db.ListToolCandidatesFromCursor(ctx, cursor)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.Empty(t, page.Cursor)

	db.SetCursorEncryptor(pagination.NewDefaultCursorEncryptor(nil))
	page, err = db.ListToolCandidates(ctx, query)
	require.Error(t, err, "an invalid AES key must fail continuation creation")
	require.Empty(t, page.Items)
	require.Empty(t, page.Cursor)
	query.Limit = MaxToolCandidateLimit
	page, err = db.ListToolCandidates(ctx, query)
	require.NoError(t, err, "a final page does not need cursor encryption")
	require.Len(t, page.Items, 2)
	require.Empty(t, page.Cursor)
}
