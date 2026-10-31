package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// GetToolSet returns the default projection of a live ToolSet.
func (s *service) GetToolSet(
	ctx context.Context,
	id apid.ID,
) (iface.ToolSet, error) {
	logical, err := s.db.GetToolSet(ctx, id)
	if err != nil {
		return nil, mapToolSetDatabaseError(err)
	}

	return s.defaultToolSetView(ctx, logical)
}

// GetToolSetGeneration returns one live generation with the current selector.
func (s *service) GetToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
) (iface.ToolSet, error) {
	logical, row, err := s.loadToolSetGeneration(ctx, id, generation)
	if err != nil {
		return nil, err
	}

	return s.hydrateToolSet(ctx, logical, row)
}

// CreateToolSet validates and compiles the first generation before creating
// anything, then creates the logical ToolSet and its generation-1 draft, and
// publishes it when primary is requested. Server-owned status is rejected.
func (s *service) CreateToolSet(
	ctx context.Context,
	resource *toolsetschema.ToolSet,
) (iface.ToolSet, error) {
	if resource == nil {
		return nil, fmt.Errorf("%w: toolset is required", ErrInvalidArgument)
	}
	if err := resource.ValidateFor(meta.ValidationModeCreate, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	id := apid.New(apid.PrefixToolSet)
	candidate := resource.ApplyAPICreateDefaults(id)

	encrypted, err := s.prepareToolSetDefinition(ctx, candidate.Metadata.Namespace, &candidate.Spec.Definition)
	if err != nil {
		return nil, err
	}

	logical := &database.ToolSet{
		Id:                 id,
		Namespace:          candidate.Metadata.Namespace,
		Name:               candidate.Metadata.Name,
		Labels:             database.Labels(candidate.Metadata.Labels),
		Annotations:        database.Annotations(candidate.Metadata.Annotations),
		ConnectionSelector: *candidate.Spec.ConnectionSelector,
	}
	if err := s.db.CreateToolSet(ctx, logical); err != nil {
		return nil, mapToolSetDatabaseError(err)
	}

	// The logical record and its first generation are separate transactions.
	// Without a generation the ToolSet could never be read, so remove it if the
	// generation cannot be created.
	row, err := s.db.CreateToolSetGeneration(ctx, id, encrypted)
	if err != nil {
		if deleteErr := s.db.DeleteToolSet(ctx, id); deleteErr != nil {
			s.logger.Error("failed to remove toolset after its first generation failed", "id", id, "error", deleteErr)
		}
		return nil, mapToolSetDatabaseError(err)
	}

	if candidate.Spec.Release.DesiredState == toolsetschema.ToolSetReleaseStatePrimary {
		if row, err = s.db.PublishToolSetGeneration(ctx, id, row.Generation, row.Revision); err != nil {
			return nil, mapToolSetDatabaseError(err)
		}
	}

	return &ToolSet{logical: *logical, generation: *row, definition: candidate.Spec.Definition}, nil
}

// UpdateToolSet applies a patch through the logical endpoint. Metadata and
// selector changes apply to the logical record and never create a generation.
// Definition or release changes edit the draft, creating one from the newest
// generation when needed, and publish it when primary is requested. Primary
// intent without a definition is a no-op while a primary exists, matching the
// ToolSet generation policy; pending drafts publish through the generation
// endpoint, or by resending their definition with primary intent.
func (s *service) UpdateToolSet(
	ctx context.Context,
	id apid.ID,
	patch *toolsetschema.ToolSetPatch,
) (iface.ToolSet, error) {
	if patch == nil {
		return nil, fmt.Errorf("%w: toolset patch is required", ErrInvalidArgument)
	}
	if patch.Metadata != nil && patch.Metadata.Generation != nil {
		return nil, fmt.Errorf("%w: metadata.generation can only be addressed through a toolset generation endpoint", ErrInvalidArgument)
	}

	logical, err := s.db.GetToolSet(ctx, id)
	if err != nil {
		return nil, mapToolSetDatabaseError(err)
	}
	current, err := s.defaultToolSetView(ctx, logical)
	if err != nil {
		return nil, err
	}

	desired, err := patch.ApplyTo(current.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	// Projected labels include system labels; only the user portion is authored.
	currentUserLabels, _ := database.SplitUserAndApxyLabels(logical.Labels)
	desiredUserLabels, _ := database.SplitUserAndApxyLabels(database.Labels(desired.Metadata.Labels))
	if desired.Metadata.Name != logical.Name ||
		!maps.Equal(currentUserLabels, desiredUserLabels) ||
		!maps.Equal(map[string]string(logical.Annotations), desired.Metadata.Annotations) {
		if logical, err = s.db.UpdateToolSetMetadata(ctx, id, desired.Metadata.Name, desiredUserLabels, desired.Metadata.Annotations); err != nil {
			return nil, mapToolSetDatabaseError(err)
		}
	}

	if patch.Spec.HasConnectionSelector() {
		selector := defaultToolSetSelector(*desired.Spec.ConnectionSelector, logical.Namespace)

		// Identical selectors are skipped so a repeated apply does not advance the
		// selector revision and restart membership work.
		if !reflect.DeepEqual(selector, logical.ConnectionSelector) {
			if logical, err = s.db.UpdateToolSetConnectionSelector(ctx, id, logical.SelectorRevision, selector); err != nil {
				return nil, mapToolSetDatabaseError(err)
			}
		}
	}

	hasDefinition := patch.Spec.HasDefinition()
	hasDesiredState := patch.Spec.HasRelease() && patch.Spec.Release.HasDesiredState()
	if !hasDefinition && (!hasDesiredState ||
		desired.Spec.Release.DesiredState == toolsetschema.ToolSetReleaseStatePrimary &&
			current.GetState() == toolsetschema.ToolSetReleaseStatePrimary) {
		return s.defaultToolSetView(ctx, logical)
	}

	draft, err := s.db.GetToolSetGenerationByState(ctx, id, toolsetschema.ToolSetReleaseStateDraft)
	if errors.Is(err, database.ErrNotFound) {
		// There is no draft to edit, so the new draft starts from the newest
		// generation's definition unless the patch replaces it.
		source, err := s.newestToolSetView(ctx, logical)
		if err != nil {
			return nil, err
		}
		definition := &source.definition
		if hasDefinition {
			definition = &desired.Spec.Definition
		}

		// desired inherits the projected generation's release intent, which is
		// primary for a published source. A new draft stays a draft unless the
		// patch itself requests publication.
		state := toolsetschema.ToolSetReleaseStateDraft
		if hasDesiredState {
			state = desired.Spec.Release.DesiredState
		}
		return s.createToolSetDraft(ctx, logical, definition, state)
	}
	if err != nil {
		return nil, mapToolSetDatabaseError(err)
	}

	draftView, err := s.hydrateToolSet(ctx, logical, draft)
	if err != nil {
		return nil, err
	}
	draftDesired, err := generationPatch(patch).ApplyTo(draftView.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return s.writeToolSetDraft(ctx, logical, draft, &draftDesired.Spec.Definition, hasDefinition, draftDesired.Spec.Release.DesiredState)
}

// CreateToolSetGeneration creates a draft from the newest generation, applying
// an optional generation patch that may replace the definition and request
// publication. Metadata and selector changes are rejected because they belong
// to the logical endpoint.
func (s *service) CreateToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	patch *toolsetschema.ToolSetPatch,
) (iface.ToolSet, error) {
	if err := validateGenerationPatch(patch); err != nil {
		return nil, err
	}

	logical, err := s.db.GetToolSet(ctx, id)
	if err != nil {
		return nil, mapToolSetDatabaseError(err)
	}

	if _, err := s.db.GetToolSetGenerationByState(
		ctx,
		id,
		toolsetschema.ToolSetReleaseStateDraft,
	); err == nil {
		return nil, ErrDraftAlreadyExists
	} else if !errors.Is(err, database.ErrNotFound) {
		return nil, mapToolSetDatabaseError(err)
	}

	source, err := s.newestToolSetView(ctx, logical)
	if err != nil {
		return nil, err
	}

	// Apply the patch to the source rendered as a draft so release intent defaults
	// to draft rather than inheriting the source's published intent.
	base := source.GetResource()
	base.Spec.Release.DesiredState = toolsetschema.ToolSetReleaseStateDraft
	base.Status.Release.State = toolsetschema.ToolSetReleaseStateDraft
	desired := base
	if patch != nil {
		if desired, err = patch.ApplyTo(base, nil); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
		}
	}

	return s.createToolSetDraft(
		ctx,
		logical,
		&desired.Spec.Definition,
		desired.Spec.Release.DesiredState,
	)
}

// UpdateToolSetGeneration patches one explicitly addressed generation. Only a
// draft may change; an empty patch or a primary request against an already
// published generation is a no-op so re-applying a published snapshot succeeds.
func (s *service) UpdateToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	patch *toolsetschema.ToolSetPatch,
) (iface.ToolSet, error) {
	if patch == nil {
		return nil, fmt.Errorf("%w: toolset patch is required", ErrInvalidArgument)
	}
	if err := validateGenerationPatch(patch); err != nil {
		return nil, err
	}

	logical, row, err := s.loadToolSetGeneration(ctx, id, generation)
	if err != nil {
		return nil, err
	}
	view, err := s.hydrateToolSet(ctx, logical, row)
	if err != nil {
		return nil, err
	}

	hasDefinition := patch.Spec.HasDefinition()
	if row.State != toolsetschema.ToolSetReleaseStateDraft {
		requestsDraft := patch.Spec.Release != nil && patch.Spec.Release.DesiredState != nil &&
			*patch.Spec.Release.DesiredState == toolsetschema.ToolSetReleaseStateDraft
		if hasDefinition || requestsDraft {
			return nil, fmt.Errorf("%w: toolset %s generation %d is %s", ErrNotDraft, id, generation, row.State)
		}
		return view, nil
	}

	desired, err := patch.ApplyTo(view.GetResource(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	return s.writeToolSetDraft(ctx, logical, row, &desired.Spec.Definition, hasDefinition, desired.Spec.Release.DesiredState)
}

// DeleteToolSet soft-deletes the logical ToolSet. Its generations become
// unreadable immediately; generated Tools are cleaned up by reconciliation.
func (s *service) DeleteToolSet(ctx context.Context, id apid.ID) error {
	return mapToolSetDatabaseError(s.db.DeleteToolSet(ctx, id))
}

// loadToolSetGeneration reads a live logical ToolSet and one of its live generations.
func (s *service) loadToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
) (*database.ToolSet, *database.ToolSetGeneration, error) {
	logical, err := s.db.GetToolSet(ctx, id)
	if err != nil {
		return nil, nil, mapToolSetDatabaseError(err)
	}

	row, err := s.db.GetToolSetGeneration(ctx, id, generation)
	if err != nil {
		return nil, nil, mapToolSetDatabaseError(err)
	}

	return logical, row, nil
}

// defaultToolSetView selects the generation a logical read shows: the primary,
// which new connections install, otherwise the draft, otherwise the newest.
func (s *service) defaultToolSetView(
	ctx context.Context,
	logical *database.ToolSet,
) (*ToolSet, error) {
	for _, state := range []toolsetschema.ToolSetReleaseState{
		toolsetschema.ToolSetReleaseStatePrimary,
		toolsetschema.ToolSetReleaseStateDraft,
	} {
		row, err := s.db.GetToolSetGenerationByState(ctx, logical.Id, state)
		if err == nil {
			return s.hydrateToolSet(ctx, logical, row)
		}
		if !errors.Is(err, database.ErrNotFound) {
			return nil, mapToolSetDatabaseError(err)
		}
	}

	return s.newestToolSetView(ctx, logical)
}

// newestToolSetView returns the highest-numbered live generation, the source a
// new draft is cloned from.
func (s *service) newestToolSetView(
	ctx context.Context,
	logical *database.ToolSet,
) (*ToolSet, error) {
	page := s.db.ListToolSetGenerationsBuilder().
		ForToolSetId(logical.Id).
		Limit(1).
		FetchPage(ctx)
	if page.Error != nil {
		return nil, mapToolSetDatabaseError(page.Error)
	}

	if len(page.Results) == 0 {
		return nil, ErrNotFound
	}

	return s.hydrateToolSet(ctx, logical, &page.Results[0])
}

// createToolSetDraft compiles a definition, stores it as a new draft, and
// publishes it when primary is requested. A concurrently created draft returns
// ErrDraftAlreadyExists.
func (s *service) createToolSetDraft(
	ctx context.Context,
	logical *database.ToolSet,
	definition *toolsetschema.ToolSetDefinition,
	desiredState toolsetschema.ToolSetReleaseState,
) (iface.ToolSet, error) {
	encrypted, err := s.prepareToolSetDefinition(ctx, logical.Namespace, definition)
	if err != nil {
		return nil, err
	}

	row, err := s.db.CreateToolSetGeneration(ctx, logical.Id, encrypted)
	if errors.Is(err, database.ErrDuplicate) {
		return nil, ErrDraftAlreadyExists
	}
	if err != nil {
		return nil, mapToolSetDatabaseError(err)
	}

	if desiredState == toolsetschema.ToolSetReleaseStatePrimary {
		if row, err = s.db.PublishToolSetGeneration(
			ctx,
			logical.Id,
			row.Generation,
			row.Revision,
		); err != nil {
			return nil, mapToolSetDatabaseError(err)
		}
	}
	return &ToolSet{logical: *logical, generation: *row, definition: *definition}, nil
}

// writeToolSetDraft replaces a draft's definition when replace is set, then
// publishes it when primary is requested. Publication always compiles the exact
// definition at the revision being promoted, so an unchanged draft is validated
// too. Stale draft revisions return ErrToolSetConflict.
func (s *service) writeToolSetDraft(
	ctx context.Context,
	logical *database.ToolSet,
	row *database.ToolSetGeneration,
	definition *toolsetschema.ToolSetDefinition,
	replace bool,
	desiredState toolsetschema.ToolSetReleaseState,
) (iface.ToolSet, error) {
	publish := desiredState == toolsetschema.ToolSetReleaseStatePrimary
	if replace || publish {
		encrypted, err := s.prepareToolSetDefinition(ctx, logical.Namespace, definition)
		if err != nil {
			return nil, err
		}
		if replace {
			if row, err = s.db.UpdateToolSetGenerationDraft(
				ctx,
				logical.Id,
				row.Generation,
				row.Revision,
				encrypted,
			); err != nil {
				return nil, mapToolSetDatabaseError(err)
			}
		}
	}

	if publish {
		var err error
		if row, err = s.db.PublishToolSetGeneration(
			ctx,
			logical.Id,
			row.Generation,
			row.Revision,
		); err != nil {
			return nil, mapToolSetDatabaseError(err)
		}
	}
	return &ToolSet{logical: *logical, generation: *row, definition: *definition}, nil
}

// prepareToolSetDefinition validates a generation definition, compiles every
// explicit template's executor without running authored code, and encrypts the
// serialized definition for the ToolSet's namespace. OpenAPI and MCP sources are
// rejected until their importers can validate them at publication.
func (s *service) prepareToolSetDefinition(
	ctx context.Context,
	ns string,
	definition *toolsetschema.ToolSetDefinition,
) (encfield.EncryptedField, error) {
	if err := definition.Validate(nil); err != nil {
		return encfield.EncryptedField{}, fmt.Errorf("%w: spec.definition: %w", ErrInvalidArgument, err)
	}
	if definition.Source.Explicit == nil {
		return encfield.EncryptedField{}, fmt.Errorf("%w: spec.definition.source: openapi and mcp sources are not supported yet; use explicit", ErrInvalidArgument)
	}

	for i := range definition.Source.Explicit.Tools {
		field := fmt.Sprintf("spec.definition.source.explicit.tools[%d].spec", i)
		if err := compileToolDefinition(
			&definition.Source.Explicit.Tools[i].Spec,
			field,
		); err != nil {
			return encfield.EncryptedField{}, err
		}
	}

	plaintext, err := json.Marshal(definition)
	if err != nil {
		return encfield.EncryptedField{}, fmt.Errorf("failed to serialize toolset definition: %w", err)
	}
	encrypted, err := s.encrypt.EncryptForNamespace(ctx, ns, plaintext)
	if err != nil {
		return encfield.EncryptedField{}, fmt.Errorf("failed to encrypt toolset definition: %w", err)
	}
	return encrypted, nil
}

// validateGenerationPatch validates a generation-endpoint patch and rejects
// logical changes, which belong to the logical endpoint.
// Metadata belongs to the logical ToolSet and selection is shared by all
// generations, so neither can be written through one generation.
func validateGenerationPatch(patch *toolsetschema.ToolSetPatch) error {
	if patch == nil {
		return nil
	}
	if err := patch.ValidateFor(meta.ValidationModeUpdate, nil); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	if patch.Spec != nil && patch.Spec.HasConnectionSelector() {
		return fmt.Errorf("%w: connection selection can only be changed through the logical toolset endpoint", ErrInvalidArgument)
	}
	if patch.Metadata != nil && *patch.Metadata != (meta.ObjectMetaPatch{}) {
		return fmt.Errorf("%w: metadata can only be changed through the logical toolset endpoint", ErrInvalidArgument)
	}
	return nil
}

// generationPatch keeps only a patch's generation fields, release and
// definition, so it can be re-applied to a draft after the logical record was
// updated. The input must already have passed validation.
func generationPatch(patch *toolsetschema.ToolSetPatch) *toolsetschema.ToolSetPatch {
	result := toolsetschema.NewToolSetPatch()
	result.Spec.Release = patch.Spec.Release
	result.Spec.Definition = patch.Spec.Definition
	return result
}

// defaultToolSetSelector fills an omitted selector namespace with the owner
// subtree, matching create defaults, so stored selectors are always explicit.
func defaultToolSetSelector(
	selector toolsetschema.ConnectionSelector,
	ns string,
) toolsetschema.ConnectionSelector {
	result := *selector.Clone()

	if result.Namespace == "" {
		result.Namespace = ns + namespace.WildcardSuffix
	}

	return result
}

// mapToolSetDatabaseError translates storage sentinels into core errors.
func mapToolSetDatabaseError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, database.ErrToolSetGenerationRevisionConflict),
		errors.Is(err, database.ErrToolSetSelectorRevisionConflict):
		return fmt.Errorf("%w: %w", ErrToolSetConflict, err)
	case errors.Is(err, database.ErrNamespaceDoesNotExist):
		return fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	default:
		return mapDatabaseError(err)
	}
}
