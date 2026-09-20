package connectors

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// connectorGenerationContext captures logical-endpoint behavior, not transport
// state. Only this policy interprets it after target selection.
type connectorGenerationContext struct {
	requiresDraft     bool
	publishDefinition bool
}

// GenerationPolicy provides the connector's draft/publication contract to the
// registry. The caller owns HTTP, comparison, pagination, and execution.
func GenerationPolicy() meta.GenerationPolicy[Connector, ConnectorPatch] {
	return meta.GenerationPolicy[Connector, ConnectorPatch]{
		State: connectorGenerationState,
		ChangesGeneration: func(p *ConnectorPatch) bool {
			return p.Spec != nil && (p.Spec.HasDefinition() || p.Spec.HasRelease())
		},
		Select:   selectConnectorGeneration,
		Finalize: finalizeConnectorGenerationPatch,
	}
}

func connectorGenerationState(c *Connector) (meta.GenerationState, error) {
	if c.Status == nil {
		return 0, fmt.Errorf("connector generation is missing release status")
	}
	switch c.Status.Release.State {
	case ConnectorReleaseStateDraft:
		return meta.GenerationEditable, nil
	case ConnectorReleaseStatePrimary:
		return meta.GenerationPublished, nil
	case ConnectorReleaseStateActive, ConnectorReleaseStateArchived:
		return meta.GenerationHistorical, nil
	default:
		return 0, fmt.Errorf("invalid connector release status")
	}
}

func selectConnectorGeneration(
	desired, selected *Connector,
	changed, hasEditable bool,
) (meta.GenerationSelection, error) {
	if !changed && (!hasEditable ||
		desired.Spec.Release.DesiredState == ConnectorReleaseStatePrimary) {
		return meta.GenerationSelection{
			Source: meta.GenerationSelected,
		}, nil
	}

	ctx := connectorGenerationContext{
		publishDefinition: desired.Spec.Release.DesiredState == ConnectorReleaseStatePrimary &&
			selected.Status != nil && selected.Status.Release.State == ConnectorReleaseStatePrimary,
	}

	if hasEditable {
		return meta.GenerationSelection{
			Source:  meta.GenerationEditableSource,
			Context: ctx,
		}, nil
	}

	ctx.requiresDraft = true

	return meta.GenerationSelection{
		Source:  meta.GenerationNewest,
		Context: ctx,
	}, nil
}

func finalizeConnectorGenerationPatch(
	desired, current *Connector,
	patch *ConnectorPatch,
	selectionContext any,
	explicit bool,
) (*ConnectorPatch, error) {
	var ctx connectorGenerationContext
	if selectionContext != nil {
		var ok bool
		ctx, ok = selectionContext.(connectorGenerationContext)
		if !ok {
			return nil, fmt.Errorf("invalid connector generation selection context")
		}
	}

	if patch.Metadata == nil || patch.Spec == nil {
		return nil, fmt.Errorf("connector patch requires metadata and spec")
	}

	// Only spec is modified; clone it and allocate replacement fields so inputs
	// (including shared live definitions and desired release intent) remain intact.
	result := *patch
	spec := *patch.Spec
	result.Spec = &spec
	state := desired.Spec.Release.DesiredState

	setRelease := func(state ConnectorReleaseState) {
		spec.Release = &ConnectorReleaseSpecPatch{DesiredState: &state}
	}

	// Historical published generations still declare primary. Logical requests
	// can publish a new generation; explicit targets never force their lifecycle.
	if !explicit &&
		state == ConnectorReleaseStatePrimary &&
		current.Status != nil &&
		current.Status.Release.State != ConnectorReleaseStatePrimary {
		setRelease(state)
	}

	// The logical endpoint considers release-only primary already satisfied if
	// another primary exists. Include the chosen draft's definition to publish it.
	if ctx.publishDefinition && !spec.HasDefinition() {
		spec.Definition = current.Spec.Definition.Clone()
	}

	if ctx.requiresDraft && !spec.HasDefinition() && !spec.HasRelease() {
		setRelease(ConnectorReleaseStateDraft)
	}

	if explicit && (*result.Metadata != (meta.ObjectMetaPatch{}) ||
			spec.HasDefinition() || spec.HasRelease()) {
		if current.Status == nil ||
			current.Status.Release.State != ConnectorReleaseStateDraft {
			return nil, fmt.Errorf("explicit connector generation is not a draft; it cannot be updated")
		}
	}

	// Definition changes may clone a draft, so retain explicit publication intent
	// even when the source generation already declared primary.
	if spec.HasDefinition() && state != "" {
		setRelease(state)
	}

	return &result, nil
}
