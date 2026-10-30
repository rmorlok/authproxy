package core

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// ToolSet is the core management view of one ToolSet generation. It pairs the
// logical record, which owns identity, metadata, and the shared connection
// selector, with one generation and its decrypted definition, so rendering the
// resource needs no further I/O. Instances are snapshots: a concurrent selector
// or draft update does not change an already loaded ToolSet.
type ToolSet struct {
	logical    database.ToolSet
	generation database.ToolSetGeneration

	// definition is the decrypted generation definition. It is trusted
	// administrator input, so it is only exposed through management resources.
	definition toolsetschema.ToolSetDefinition
}

func (t *ToolSet) GetId() apid.ID                { return t.logical.Id }
func (t *ToolSet) GetNamespace() string          { return t.logical.Namespace }
func (t *ToolSet) GetName() scommon.ResourceName { return t.logical.Name }
func (t *ToolSet) GetGeneration() uint64         { return t.generation.Generation }

// GetState returns the generation's observed release state.
func (t *ToolSet) GetState() toolsetschema.ToolSetReleaseState { return t.generation.State }

// GetResource renders a detached canonical resource. The selector is always the
// current logical selector, never a historical one, because selection is shared
// across generations. Published generations keep primary release intent after
// they become active or archived.
func (t *ToolSet) GetResource() *toolsetschema.ToolSet {
	createdAt := t.logical.CreatedAt

	// Either a logical metadata/selector write or a draft edit counts as a change
	// to the projected resource.
	updatedAt := t.logical.UpdatedAt
	if t.generation.UpdatedAt.After(updatedAt) {
		updatedAt = t.generation.UpdatedAt
	}

	desired := toolsetschema.ToolSetReleaseStatePrimary
	if t.generation.State == toolsetschema.ToolSetReleaseStateDraft {
		desired = toolsetschema.ToolSetReleaseStateDraft
	}

	return &toolsetschema.ToolSet{
		TypeMeta: meta.NewTypeMeta(toolsetschema.ToolSetKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          t.logical.Id.String(),
			Name:        t.logical.Name,
			Namespace:   t.logical.Namespace,
			Generation:  t.generation.Generation,
			Labels:      maps.Clone(map[string]string(t.logical.Labels)),
			Annotations: maps.Clone(map[string]string(t.logical.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: toolsetschema.ToolSetSpec{
			ConnectionSelector: t.logical.ConnectionSelector.Clone(),
			Release:            toolsetschema.ToolSetReleaseSpec{DesiredState: desired},
			Definition:         *t.definition.Clone(),
		},
		Status: &toolsetschema.ToolSetStatus{
			Release: toolsetschema.ToolSetReleaseStatus{State: t.generation.State},
		},
	}
}

var _ iface.ToolSet = (*ToolSet)(nil)

// hydrateToolSet decrypts one generation's definition and pairs it with the
// supplied logical record. The generation must belong to that record.
func (s *service) hydrateToolSet(
	ctx context.Context,
	logical *database.ToolSet,
	generation *database.ToolSetGeneration,
) (*ToolSet, error) {
	plaintext, err := s.encrypt.Decrypt(ctx, generation.EncryptedDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt toolset %s generation %d: %w", logical.Id, generation.Generation, err)
	}

	// ToolSetDefinition's decoder is strict, rejecting unknown or null fields.
	var definition toolsetschema.ToolSetDefinition
	if err := json.Unmarshal(plaintext, &definition); err != nil {
		return nil, fmt.Errorf("failed to decode toolset %s generation %d: %w", logical.Id, generation.Generation, err)
	}

	return &ToolSet{
		logical:    *logical,
		generation: *generation,
		definition: definition,
	}, nil
}
