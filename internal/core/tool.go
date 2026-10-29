package core

import (
	"context"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
)

// Tool is the core management view of a database Tool. It is hydrated with the
// decrypted active definition and, for generated Tools, the owning ToolSet
// generation, so rendering the resource needs no further I/O. Instances are
// snapshots: a concurrent update does not change an already loaded Tool.
type Tool struct {
	database.Tool

	// definition is the decrypted active revision. It is trusted administrator
	// input and may reference connection configuration, so it is only exposed
	// through management resources, never discovery descriptors.
	definition toolschema.ToolDefinition

	// managedBy is nil for standalone Tools and identifies the owning ToolSet,
	// applied generation, and source key for generated Tools.
	managedBy *toolschema.ToolManagedBy
}

func (t *Tool) GetId() apid.ID                { return t.Id }
func (t *Tool) GetNamespace() string          { return t.Namespace }
func (t *Tool) GetName() scommon.ResourceName { return t.Name }
func (t *Tool) GetConnectionId() apid.ID      { return t.ConnectionId }
func (t *Tool) GetActiveRevision() uint64     { return t.ActiveRevision }

// GetManagedByToolSetId returns the owning ToolSet ID, or apid.Nil when standalone.
func (t *Tool) GetManagedByToolSetId() apid.ID {
	if t.managedBy == nil {
		return apid.Nil
	}
	id, err := apid.Parse(t.managedBy.ToolSetRef.ID)
	if err != nil {
		return apid.Nil
	}
	return id
}

// GetResource renders a detached canonical resource. The connection reference is
// always the resolved ID, even if the author originally referenced it by name.
// Readiness conditions are omitted until invocation admission observes them.
func (t *Tool) GetResource() *toolschema.Tool {
	createdAt := t.CreatedAt
	updatedAt := t.UpdatedAt
	resource := &toolschema.Tool{
		TypeMeta: meta.NewTypeMeta(toolschema.ToolKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          t.Id.String(),
			Name:        t.Name,
			Namespace:   t.Namespace,
			Labels:      maps.Clone(map[string]string(t.Labels)),
			Annotations: maps.Clone(map[string]string(t.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: toolschema.ToolSpec{
			ConnectionRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       connectionschema.ConnectionKind,
				ID:         t.ConnectionId.String(),
			},
			ToolDefinition: *t.definition.Clone(),
		},
		Status: &toolschema.ToolStatus{
			Revision: t.ActiveRevision,
		},
	}
	if t.managedBy != nil {
		managedBy := *t.managedBy
		resource.Status.ManagedBy = &managedBy
	}
	return resource
}

var _ iface.Tool = (*Tool)(nil)

// hydrateTool decrypts a Tool's active revision and resolves generated ownership.
// It reads the revision by number rather than the admission snapshot so Tools
// whose connection was deleted remain inspectable and deletable.
func (s *service) hydrateTool(ctx context.Context, tool *database.Tool) (*Tool, error) {
	revision, err := s.db.GetToolRevision(ctx, tool.Id, tool.ActiveRevision)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	plaintext, err := s.encrypt.Decrypt(ctx, revision.EncryptedDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt tool %s revision %d: %w", tool.Id, revision.Revision, err)
	}

	var definition toolschema.ToolDefinition
	if err := util.DecodeJSONStrict(plaintext, &definition); err != nil {
		return nil, fmt.Errorf("failed to decode tool %s revision %d: %w", tool.Id, revision.Revision, err)
	}

	result := &Tool{Tool: *tool, definition: definition}

	if !tool.BindingId.IsNil() {
		binding, err := s.db.GetToolSetBinding(ctx, tool.BindingId)
		if err != nil {
			return nil, mapDatabaseError(err)
		}

		// The revision captures the generation it was prepared from; fall back to
		// the binding's applied generation for rows that predate that capture.
		var generation uint64
		if revision.SourceGeneration != nil {
			generation = *revision.SourceGeneration
		} else if binding.AppliedGeneration != nil {
			generation = *binding.AppliedGeneration
		}

		result.managedBy = &toolschema.ToolManagedBy{
			ToolSetRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       toolsetschema.ToolSetKind,
				ID:         binding.ToolSetId.String(),
				Generation: generation,
			},
			SourceKey: tool.SourceKey,
		}
	}

	return result, nil
}
