package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// defaultToolListLimit is the page size used when a list request omits one.
const defaultToolListLimit = 50

// toolPublishRenderLimits bound request-template compilation at publication.
// They only reject definitions too large or deep to compile; invocation will
// recompile under its own effective execution policy.
var toolPublishRenderLimits = render.Limits{
	MaxBytes: 1 << 20,
	MaxDepth: 64,
	MaxSteps: 1_000_000,
}

// toolPublishResponseLimits bound response-rule and transform compilation at
// publication. The byte limits match the design's proposed invocation defaults;
// compiling does not run the transform or allocate these budgets.
var toolPublishResponseLimits = httpcodec.ResponseLimits{
	MaxSourceBytes:        256 << 10,
	MaxResponseBytes:      8 << 20,
	MaxOutputBytes:        8 << 20,
	MaxEncodedResultBytes: 12 << 20,
	Transform: javascript.Limits{
		MaxSourceBytes: 64 << 10,
		MaxInputBytes:  8 << 20,
		MaxOutputBytes: 8 << 20,
		MaxDepth:       64,
		MaxNodes:       1_000_000,
		MaxStackDepth:  256,
		Timeout:        5 * time.Second,
	},
}

// preparedToolDefinition is a compiled, encrypted definition ready to publish
// as an immutable revision, together with the projections stored beside it.
type preparedToolDefinition struct {
	encrypted encfield.EncryptedField
	digest    string
}

// GetTool returns the management view of a live Tool, standalone or generated.
func (s *service) GetTool(ctx context.Context, id apid.ID) (iface.Tool, error) {
	tool, err := s.db.GetTool(ctx, id)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	return s.hydrateTool(ctx, tool)
}

// CreateTool validates and compiles a standalone Tool, resolves its connection
// within the Tool's namespace, and publishes revision 1. Server-owned status,
// including generated ownership, is rejected by create validation.
func (s *service) CreateTool(ctx context.Context, resource *toolschema.Tool) (iface.Tool, error) {
	if resource == nil {
		return nil, fmt.Errorf("%w: tool is required", ErrInvalidArgument)
	}
	if err := resource.ValidateFor(meta.ValidationModeCreate, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	id := apid.New(apid.PrefixTool)
	candidate := resource.ApplyCreateDefaults(id)

	connectionId, err := s.resolveToolConnection(ctx, candidate)
	if err != nil {
		return nil, err
	}

	prepared, err := s.prepareToolDefinition(ctx, candidate)
	if err != nil {
		return nil, err
	}

	tool := &database.Tool{
		Id:           id,
		Namespace:    candidate.Metadata.Namespace,
		Name:         candidate.Metadata.Name,
		ConnectionId: connectionId,
		Labels:       database.Labels(maps.Clone(candidate.Metadata.Labels)),
		Annotations:  database.Annotations(maps.Clone(candidate.Metadata.Annotations)),
		Description:  candidate.Spec.Description,
		Verbs:        candidate.Spec.Verbs,
	}
	if _, err := s.db.CreateStandaloneTool(ctx, tool, prepared.encrypted, prepared.digest); err != nil {
		return nil, mapDatabaseError(err)
	}

	return &Tool{Tool: *tool, definition: *candidate.Spec.ToolDefinition.Clone()}, nil
}

// UpdateTool publishes a complete replacement as the next revision. The
// candidate is checked against the currently stored resource so identity,
// namespace, and status cannot change, then compiled before any write. The
// write itself compares expectedRevision atomically, so a concurrent update
// returns ErrToolRevisionConflict and leaves the newer revision active.
func (s *service) UpdateTool(
	ctx context.Context,
	id apid.ID,
	expectedRevision uint64,
	resource *toolschema.Tool,
) (iface.Tool, error) {
	if resource == nil {
		return nil, fmt.Errorf("%w: tool is required", ErrInvalidArgument)
	}

	current, err := s.db.GetTool(ctx, id)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if !current.BindingId.IsNil() {
		return nil, s.toolManagedError(ctx, current)
	}
	if current.ActiveRevision != expectedRevision {
		return nil, ErrToolRevisionConflict
	}

	before, err := s.hydrateTool(ctx, current)
	if err != nil {
		return nil, err
	}
	if err := toolschema.ValidateUpdate(before.GetResource(), resource, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	candidate := resource.Clone()
	if err := candidate.Spec.ValidateForNamespace(candidate.Metadata.Namespace, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	connectionId, err := s.resolveToolConnection(ctx, candidate)
	if err != nil {
		return nil, err
	}

	prepared, err := s.prepareToolDefinition(ctx, candidate)
	if err != nil {
		return nil, err
	}

	updated := *current
	updated.Name = candidate.Metadata.Name
	updated.ConnectionId = connectionId
	updated.Labels = database.Labels(maps.Clone(candidate.Metadata.Labels))
	updated.Annotations = database.Annotations(maps.Clone(candidate.Metadata.Annotations))
	updated.Description = candidate.Spec.Description
	updated.Verbs = candidate.Spec.Verbs
	if _, err := s.db.UpdateStandaloneTool(ctx, &updated, expectedRevision, prepared.encrypted, prepared.digest); err != nil {
		return nil, mapDatabaseError(err)
	}

	return &Tool{Tool: updated, definition: *candidate.Spec.ToolDefinition.Clone()}, nil
}

// DeleteTool soft-deletes a standalone Tool if expectedRevision is still active.
// History is retained, and the name becomes available for a new Tool.
func (s *service) DeleteTool(ctx context.Context, id apid.ID, expectedRevision uint64) error {
	current, err := s.db.GetTool(ctx, id)
	if err != nil {
		return mapDatabaseError(err)
	}
	if !current.BindingId.IsNil() {
		return s.toolManagedError(ctx, current)
	}
	if err := s.db.DeleteStandaloneTool(ctx, id, expectedRevision); err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

// ListTools returns one hydrated page of management Tools. The namespace scope
// is required so callers always constrain the listing by their permissions.
func (s *service) ListTools(ctx context.Context, req iface.ListToolsRequest) (iface.ToolPage, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = defaultToolListLimit
	}
	if limit > database.MaxToolListLimit {
		limit = database.MaxToolListLimit
	}

	page, err := s.db.ListTools(ctx, database.ToolListQuery{
		NamespaceMatchers: req.NamespaceMatchers,
		Name:              req.Name,
		ConnectionId:      req.ConnectionId,
		LabelSelector:     req.LabelSelector,
		Limit:             limit,
		OrderByField:      req.OrderByField,
		OrderBy:           req.OrderBy,
	})
	if err != nil {
		return iface.ToolPage{}, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return s.hydrateToolPage(ctx, page)
}

// ListToolsFromCursor continues a listing with the filters saved in the cursor.
func (s *service) ListToolsFromCursor(ctx context.Context, cursor string) (iface.ToolPage, error) {
	page, err := s.db.ListToolsFromCursor(ctx, cursor)
	if err != nil {
		return iface.ToolPage{}, fmt.Errorf("%w: invalid cursor: %w", ErrInvalidArgument, err)
	}
	return s.hydrateToolPage(ctx, page)
}

// hydrateToolPage decrypts each listed Tool's active definition.
func (s *service) hydrateToolPage(ctx context.Context, page database.ToolListPage) (iface.ToolPage, error) {
	result := iface.ToolPage{
		Items:  make([]iface.Tool, 0, len(page.Items)),
		Cursor: page.Cursor,
	}
	for i := range page.Items {
		tool, err := s.hydrateTool(ctx, &page.Items[i])
		if err != nil {
			return iface.ToolPage{}, err
		}
		result.Items = append(result.Items, tool)
	}
	return result, nil
}

// resolveToolConnection resolves an ID or namespace/name connection reference
// and requires the live connection to be in the Tool's own namespace. A Tool
// can never bind a connection elsewhere, even an ancestor or descendant.
func (s *service) resolveToolConnection(ctx context.Context, tool *toolschema.Tool) (apid.ID, error) {
	ref := tool.Spec.ConnectionRef
	if ref.ID == "" && ref.Namespace == "" {
		// A name-only reference is relative to the Tool's namespace.
		ref.Namespace = tool.Metadata.Namespace
	}

	connection, err := s.db.ResolveConnectionReference(ctx, ref)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return apid.Nil, fmt.Errorf("%w: connection referenced by spec.connectionRef was not found", ErrInvalidArgument)
		}
		return apid.Nil, coreObjectReferenceError(ref.Kind, err)
	}
	if connection.Namespace != tool.Metadata.Namespace {
		return apid.Nil, fmt.Errorf(
			"%w: spec.connectionRef must be in the tool namespace %q",
			ErrInvalidArgument,
			tool.Metadata.Namespace,
		)
	}

	return connection.Id, nil
}

// prepareToolDefinition validates and compiles the definition without
// executing authored code or making requests, then serializes and encrypts it
// for the Tool's namespace. The digest covers the canonical plaintext so it is
// stable across randomized encryption and later key rotation.
func (s *service) prepareToolDefinition(ctx context.Context, tool *toolschema.Tool) (*preparedToolDefinition, error) {
	definition := &tool.Spec.ToolDefinition
	if err := compileToolDefinition(definition, "spec"); err != nil {
		return nil, err
	}

	plaintext, err := json.Marshal(definition)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tool definition: %w", err)
	}

	encrypted, err := s.encrypt.EncryptForNamespace(ctx, tool.Metadata.Namespace, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt tool definition: %w", err)
	}

	digest := sha256.Sum256(plaintext)
	return &preparedToolDefinition{
		encrypted: encrypted,
		digest:    hex.EncodeToString(digest[:]),
	}, nil
}

// compileToolDefinition validates a Tool definition and compiles its executor
// without running authored code or making requests. field prefixes error paths,
// such as "spec" for a standalone Tool or a ToolSet template's spec path.
// Failures wrap ErrInvalidArgument.
func compileToolDefinition(
	definition *toolschema.ToolDefinition,
	field string,
) error {
	if err := definition.Validate(nil); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidArgument, field, err)
	}

	switch {
	case definition.ProxyHTTP != nil:
		if _, err := httpcodec.CompileRequest(
			definition.ProxyHTTP,
			toolPublishRenderLimits,
		); err != nil {
			return fmt.Errorf("%w: %s.proxyHttp: %w", ErrInvalidArgument, field, err)
		}
		if _, err := httpcodec.CompileResponse(
			definition.ProxyHTTP.Response,
			toolPublishResponseLimits,
		); err != nil {
			return fmt.Errorf("%w: %s.proxyHttp.response: %w", ErrInvalidArgument, field, err)
		}
	case definition.Javascript != nil:
		// The execute(params, context) host is not implemented yet, so there is
		// no compiler that can validate an entry point. Refuse rather than store
		// a definition that has never been compiled.
		return fmt.Errorf("%w: %s.javascript tools are not supported yet; use %s.proxyHttp", ErrInvalidArgument, field, field)
	default:
		return fmt.Errorf("%w: %s must declare an executor", ErrInvalidArgument, field)
	}

	return nil
}

// toolManagedError identifies the owning ToolSet so callers know where to make
// the change. It falls back to the bare sentinel if the binding is unreadable.
func (s *service) toolManagedError(ctx context.Context, tool *database.Tool) error {
	binding, err := s.db.GetToolSetBinding(ctx, tool.BindingId)
	if err != nil {
		return ErrToolManaged
	}
	return fmt.Errorf("%w: update ToolSet %s instead", ErrToolManaged, binding.ToolSetId)
}
