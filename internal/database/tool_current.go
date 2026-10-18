package database

import (
	sq "github.com/Masterminds/squirrel"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// currentToolOwnershipCondition checks the source of a current Tool definition
// in the same SQL snapshot as its metadata or ciphertext. The caller supplies
// aliases t for a live Tool and r for its live active revision, and checks their
// shared live connection and namespace. Standalone revisions have no source
// generation; generated revisions must belong to a complete installed inventory.
//
// This is storage consistency, not authorization or execution readiness. Current
// selector matching is deliberately absent because membership removal happens
// through reconciliation. Archived generations remain usable by existing pins;
// new assignments require primary, and migrations require primary or active.
func (s *service) currentToolOwnershipCondition() sq.Sqlizer {
	installed := s.sq.Select("1").
		From(ToolSetBindingsTable + " b").
		Join(ToolSetsTable + " ts ON ts.id = b.tool_set_id").
		Join(NamespacesTable + " tsn ON tsn.path = ts.namespace").
		Join(ToolSetGenerationsTable + " g ON g.tool_set_id = b.tool_set_id AND g.generation = b.applied_generation").
		Where("b.id = t.binding_id").
		Where("b.connection_id = t.connection_id").
		Where("b.applied_generation = r.source_generation").
		Where(sq.Gt{"b.inventory_revision": 0}).
		Where(sq.Eq{
			"b.deleted_at":   nil,
			"ts.deleted_at":  nil,
			"tsn.deleted_at": nil,
			"g.deleted_at":   nil,
			"g.state": []toolsetschema.ToolSetReleaseState{
				toolsetschema.ToolSetReleaseStatePrimary,
				toolsetschema.ToolSetReleaseStateActive,
				toolsetschema.ToolSetReleaseStateArchived,
			},
		}).
		// The outer query numbers placeholders after composing the subquery.
		PlaceholderFormat(sq.Question)

	return sq.Or{
		sq.Eq{"t.binding_id": nil, "r.source_generation": nil},
		sq.Expr("EXISTS (?)", installed),
	}
}
