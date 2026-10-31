package database

import (
	"context"
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolExecutionSnapshot captures the stored inputs for one Tool invocation in a
// single SQL read. It is internal storage data, not an authorized invocation or
// an API response. The caller must authorize the captured revision's aliases,
// check any expected revision and execution readiness, and decrypt/compile the
// captured definitions before using them. Credentials are resolved separately
// through the connection's authentication method and may still refresh.
//
// Each read owns its maps, pointers, and ciphertext. Later database updates do
// not change a returned snapshot. Consumers must likewise avoid rereading the
// active Tool or connection and mixing those new values with this revision.
type ToolExecutionSnapshot struct {
	// Revision is the active Tool definition, including its schemas, aliases,
	// and captured source generation. It belongs to Connection.Id.
	Revision ToolRevision

	// Connection captures configuration ciphertext, effective stored labels,
	// annotations, lifecycle/setup/health state, and the pinned connector
	// identity. For example, a tenant configuration changed after this read does
	// not replace the tenant used by this invocation's templates.
	Connection Connection

	// Connector is the exact generation referenced by Connection, with its
	// logical metadata and encrypted definition. Hydration can build the
	// connection client from these rows without rereading its generation pin.
	Connector ConnectorWithDefinition

	// ToolSetId identifies the live installed binding's owner for a generated
	// Tool. It is apid.Nil for a standalone Tool; Revision.SourceGeneration
	// supplies the corresponding ToolSet generation.
	ToolSetId apid.ID
}

// GetToolExecutionSnapshot returns one consistent active revision, connection,
// pinned connector definition, and source identity. Missing or soft-deleted
// records and namespaces return ErrNotFound. Generated Tools must also satisfy
// the installed-binding checks used by GetCurrentToolRevision.
//
// Live means not soft-deleted here: lifecycle states, setup, health, permissions,
// and expectedRevision are admission decisions. Selector changes take effect
// through reconciliation rather than this read. This method neither locks rows
// beyond the read nor freezes credentials for the eventual invocation.
func (s *service) GetToolExecutionSnapshot(ctx context.Context, id apid.ID) (*ToolExecutionSnapshot, error) {
	if err := validateToolID(id); err != nil {
		return nil, err
	}

	var result ToolExecutionSnapshot
	columns := util.PrependStringToAll("r.", result.Revision.cols())
	columns = append(columns, util.PrependStringToAll("cxn.", result.Connection.cols())...)
	columns = append(columns, connectorWithDefinitionSelectCols()...)
	columns = append(columns, "source_binding.tool_set_id")
	fields := append(result.Revision.fields(), result.Connection.fields()...)
	fields = append(fields, result.Connector.fields()...)
	fields = append(fields, &result.ToolSetId)

	// All mutable pointers and their dependent records are read by the same
	// statement. Separate GetConnection/GetConnectorGeneration calls could
	// otherwise combine an older Tool definition with a newly migrated client.
	err := s.sq.Select(columns...).
		From(ToolsTable + " t").
		Join(ToolRevisionsTable + " r ON r.tool_id = t.id AND r.revision = t.active_revision").
		Join(ConnectionsTable + " cxn ON cxn.id = t.connection_id AND cxn.id = r.connection_id AND cxn.namespace = t.namespace").
		Join(NamespacesTable + " n ON n.path = cxn.namespace").
		Join(ConnectorsTable + " c ON c.id = cxn.connector_id").
		Join(ConnectorGenerationsTable + " dv ON dv.connector_id = c.id AND dv.generation = cxn.connector_generation").
		Join(NamespacesTable + " cn ON cn.path = c.namespace").
		LeftJoin(ToolSetBindingsTable + " source_binding ON source_binding.id = t.binding_id").
		Where(sq.Eq{
			"t.id":           id,
			"t.deleted_at":   nil,
			"r.deleted_at":   nil,
			"cxn.deleted_at": nil,
			"n.deleted_at":   nil,
			"c.deleted_at":   nil,
			"dv.deleted_at":  nil,
			"cn.deleted_at":  nil,
		}).
		Where(s.currentToolOwnershipCondition()).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(fields...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}
