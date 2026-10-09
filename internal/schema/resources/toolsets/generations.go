package toolsets

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// toolSetGenerationContext retains generation work identified against the
// logical endpoint even when comparing the chosen draft later produces no diff.
// A nil context means no generation source has been selected yet.
type toolSetGenerationContext struct {
	requiresDraft     bool
	publishDefinition bool
}

// GenerationPolicy supplies pure draft/publication rules for future registry
// integration. Selection changes affect the logical ToolSet, never a generation
// or an existing connection's pinned generation. Callers own I/O, comparison,
// patch validation, and publication; none of these functions changes status.
func GenerationPolicy() meta.GenerationPolicy[ToolSet, ToolSetPatch] {
	return meta.GenerationPolicy[ToolSet, ToolSetPatch]{
		State:             toolSetGenerationState,
		ChangesGeneration: toolSetPatchChangesGeneration,
		Select:            selectToolSetGeneration,
		Finalize:          finalizeToolSetGenerationPatch,
	}
}

// toolSetGenerationState classifies observed release state without inferring
// writability from desired intent. Missing or unfamiliar states fail closed.
func toolSetGenerationState(resource *ToolSet) (meta.GenerationState, error) {
	if resource == nil || resource.Status == nil {
		return 0, fmt.Errorf("tool set generation is missing release status")
	}

	switch resource.Status.Release.State {
	case ToolSetReleaseStateDraft:
		return meta.GenerationEditable, nil
	case ToolSetReleaseStatePrimary:
		return meta.GenerationPublished, nil
	case ToolSetReleaseStateActive, ToolSetReleaseStateArchived:
		return meta.GenerationHistorical, nil
	default:
		return 0, fmt.Errorf("invalid tool set release status")
	}
}

// toolSetPatchChangesGeneration ignores logical metadata, selection, and an
// empty release object. Invalid explicit null definition/desired-state values
// retain their presence so ordinary patch validation can reject them.
func toolSetPatchChangesGeneration(patch *ToolSetPatch) bool {
	return patch != nil && patch.Spec != nil &&
		(patch.Spec.HasDefinition() || patch.Spec.Release.HasDesiredState())
}

// selectToolSetGeneration keeps non-generation updates on the selected snapshot.
// Choosing an unrelated draft would cause a subsequent diff to rewrite its
// definition merely because the caller changed the shared selector or metadata.
func selectToolSetGeneration(
	desired, selected *ToolSet,
	changesGeneration, hasEditable bool,
) (meta.GenerationSelection, error) {
	if desired == nil {
		return meta.GenerationSelection{}, fmt.Errorf("desired tool set is required")
	}

	if _, err := toolSetGenerationState(selected); err != nil {
		return meta.GenerationSelection{}, err
	}

	if !changesGeneration {
		return meta.GenerationSelection{Source: meta.GenerationSelected}, nil
	}

	ctx := toolSetGenerationContext{
		publishDefinition: desired.Spec.Release.DesiredState == ToolSetReleaseStatePrimary &&
			selected.Status.Release.State == ToolSetReleaseStatePrimary,
	}

	if hasEditable {
		return meta.GenerationSelection{Source: meta.GenerationEditableSource, Context: ctx}, nil
	}

	ctx.requiresDraft = true
	return meta.GenerationSelection{Source: meta.GenerationNewest, Context: ctx}, nil
}

// finalizeToolSetGenerationPatch returns a detached patch with publication intent
// retained after comparison. A preliminary CLI comparison also calls this before
// Select, so nil-context logical-only patches must never acquire generation work.
// Explicit targets reject selector presence and writes to non-draft generations;
// callers must prune equal-valued fields before treating a patch as a no-op.
func finalizeToolSetGenerationPatch(
	desired, current *ToolSet,
	patch *ToolSetPatch,
	selectionContext any,
	explicit bool,
) (*ToolSetPatch, error) {
	if desired == nil {
		return nil, fmt.Errorf("desired tool set is required")
	}

	state, err := toolSetGenerationState(current)
	if err != nil {
		return nil, err
	}

	if patch == nil || patch.Metadata == nil || patch.Spec == nil {
		return nil, fmt.Errorf("tool set patch requires metadata and spec")
	}

	var ctx toolSetGenerationContext
	if selectionContext != nil {
		var ok bool
		ctx, ok = selectionContext.(toolSetGenerationContext)
		if !ok || explicit {
			return nil, fmt.Errorf("invalid tool set generation selection context")
		}
	}

	if explicit {
		if patch.Spec.HasConnectionSelector() {
			return nil, fmt.Errorf("connection selection can only be changed through the logical tool set endpoint")
		}
		if state != meta.GenerationEditable &&
			(*patch.Metadata != (meta.ObjectMetaPatch{}) || toolSetPatchChangesGeneration(patch)) {
			return nil, fmt.Errorf("explicit tool set generation is not a draft; it cannot be updated")
		}
	}

	result := patch.Clone()
	if !toolSetPatchChangesGeneration(result) && selectionContext == nil {
		return result, nil
	}

	setRelease := func(state ToolSetReleaseState) {
		result.Spec.Release = &ToolSetReleaseSpecPatch{DesiredState: &state}
	}

	desiredState := desired.Spec.Release.DesiredState
	if !explicit && desiredState == ToolSetReleaseStatePrimary && state != meta.GenerationPublished {
		setRelease(desiredState)
	}

	// A logical release-only primary update may already be satisfied by another
	// primary. Including the chosen definition requests publication of that draft.
	if ctx.publishDefinition && !result.Spec.HasDefinition() {
		result.Spec.Definition = current.Spec.Definition.Clone()
	}

	if ctx.requiresDraft && !toolSetPatchChangesGeneration(result) {
		setRelease(ToolSetReleaseStateDraft)
	}

	if result.Spec.HasDefinition() && desiredState != "" {
		setRelease(desiredState)
	}

	return result, nil
}
