package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// connectionToolSetBinding adapts one stored assignment to its management view.
type connectionToolSetBinding struct {
	row database.ToolSetBindingListing
}

// GetConnectionId returns the connection holding the assignment.
func (b *connectionToolSetBinding) GetConnectionId() apid.ID {
	return b.row.ConnectionId
}

// GetToolSetId returns the logical ToolSet assigned to the connection.
func (b *connectionToolSetBinding) GetToolSetId() apid.ID {
	return b.row.ToolSetId
}

// GetToolSetNamespace returns the namespace that owns the logical ToolSet.
func (b *connectionToolSetBinding) GetToolSetNamespace() string {
	return b.row.ToolSetNamespace
}

// GetResource projects the assignment without its internal binding ID, epoch, or
// inventory revision. Installed is the only stored observation so far: Ready needs
// a live connection check, and MCP catalog status has no storage until the MCP
// importer lands, so both are omitted rather than guessed.
func (b *connectionToolSetBinding) GetResource() *schemaapi.ConnectionToolSetBinding {
	resource := schemaapi.NewConnectionToolSetBinding(
		b.row.ConnectionNamespace,
		meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       connectionschema.ConnectionKind,
			ID:         b.row.ConnectionId.String(),
			Namespace:  b.row.ConnectionNamespace,
		},
		meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       toolsetschema.ToolSetKind,
			ID:         b.row.ToolSetId.String(),
			Name:       b.row.ToolSetName,
			Namespace:  b.row.ToolSetNamespace,
		},
	)

	installed := meta.Condition{
		Type:               schemaapi.ToolSetBindingConditionInstalled,
		Status:             meta.ConditionFalse,
		LastTransitionTime: b.row.UpdatedAt,
		Reason:             schemaapi.ToolSetBindingReasonInventoryPending,
	}
	if b.row.AppliedGeneration != nil {
		generation := *b.row.AppliedGeneration
		resource.Status.AppliedGeneration = &generation
		installed.Status = meta.ConditionTrue
		installed.ObservedGeneration = generation
		installed.Reason = schemaapi.ToolSetBindingReasonInventoryInstalled
	}
	resource.Status.Conditions = []meta.Condition{installed}

	return resource
}

// listConnectionToolSetBindingsWrapper adapts a database binding listing to
// management views. Exactly one of l or e is set: l while building a listing, e
// when resuming a cursor.
type listConnectionToolSetBindingsWrapper struct {
	l database.ListToolSetBindingsBuilder
	e database.ListToolSetBindingsExecutor
}

// convertPageResult wraps each row. No per-row reads or decryption are needed.
func (l *listConnectionToolSetBindingsWrapper) convertPageResult(
	result pagination.PageResult[database.ToolSetBindingListing],
) pagination.PageResult[iface.ConnectionToolSetBinding] {
	if result.Error != nil {
		return pagination.PageResult[iface.ConnectionToolSetBinding]{Error: mapToolListError(result.Error)}
	}

	items := make([]iface.ConnectionToolSetBinding, 0, len(result.Results))
	for i := range result.Results {
		items = append(items, &connectionToolSetBinding{row: result.Results[i]})
	}

	return pagination.PageResult[iface.ConnectionToolSetBinding]{
		Results: items,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listConnectionToolSetBindingsWrapper) executor() database.ListToolSetBindingsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listConnectionToolSetBindingsWrapper) FetchPage(
	ctx context.Context,
) pagination.PageResult[iface.ConnectionToolSetBinding] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listConnectionToolSetBindingsWrapper) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[iface.ConnectionToolSetBinding],
) error {
	err := l.executor().Enumerate(ctx, func(result pagination.PageResult[database.ToolSetBindingListing]) (pagination.KeepGoing, error) {
		return callback(l.convertPageResult(result))
	})
	return mapToolListError(err)
}

func (l *listConnectionToolSetBindingsWrapper) Limit(limit int32) iface.ListConnectionToolSetBindingsBuilder {
	return &listConnectionToolSetBindingsWrapper{l: l.l.Limit(limit)}
}

func (l *listConnectionToolSetBindingsWrapper) ForConnectionId(id apid.ID) iface.ListConnectionToolSetBindingsBuilder {
	return &listConnectionToolSetBindingsWrapper{l: l.l.ForConnectionId(id)}
}

func (l *listConnectionToolSetBindingsWrapper) ForToolSetNamespaceMatchers(
	matchers []string,
) iface.ListConnectionToolSetBindingsBuilder {
	return &listConnectionToolSetBindingsWrapper{l: l.l.ForToolSetNamespaceMatchers(matchers)}
}

// ListConnectionToolSetBindingsBuilder starts a listing of one connection's assignments.
func (s *service) ListConnectionToolSetBindingsBuilder() iface.ListConnectionToolSetBindingsBuilder {
	return &listConnectionToolSetBindingsWrapper{l: s.db.ListToolSetBindingsBuilder()}
}

// ListConnectionToolSetBindingsFromCursor continues a listing with its saved filters.
func (s *service) ListConnectionToolSetBindingsFromCursor(
	ctx context.Context,
	cursor string,
) (iface.ListConnectionToolSetBindingsExecutor, error) {
	e, err := s.db.ListToolSetBindingsFromCursor(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return &listConnectionToolSetBindingsWrapper{e: e}, nil
}

var _ iface.ListConnectionToolSetBindingsBuilder = (*listConnectionToolSetBindingsWrapper)(nil)
