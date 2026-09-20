package registry

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// GenerationLifecycle is optional on ResourceType. It exposes resource-owned
// lifecycle rules without requiring callers to know concrete resource types,
// release field paths, state names, or generation-producing fields. It assumes
// the shared metadata.generation and collection/:id/generations API contract,
// with at most one editable generation. Calls reject wrong types and typed nils.
type GenerationLifecycle interface {
	State(resource any) (meta.GenerationState, error)
	ChangesGeneration(patch any) (bool, error)
	Select(
		desired, selected any,
		changesGeneration, hasEditable bool,
	) (meta.GenerationSelection, error)
	Finalize(desired, current, patch any,
		selectionContext any,
		explicit bool,
	) (any, error)
}

type generationLifecycle[R, P any] struct {
	policy meta.GenerationPolicy[R, P]
}

func withGenerations[R, P any](
	resource ResourceType,
	policy meta.GenerationPolicy[R, P],
) ResourceType {
	resource.Generations = generationLifecycle[R, P]{policy: policy}
	return resource
}

func generationResource[R any](value any) (*R, error) {
	r, ok := value.(*R)
	if !ok || r == nil {
		return nil, fmt.Errorf("unexpected generation resource type")
	}

	return r, nil
}

func (g generationLifecycle[R, P]) State(
	resource any,
) (meta.GenerationState, error) {
	r, err := generationResource[R](resource)
	if err != nil {
		return 0, err
	}

	return g.policy.State(r)
}

func (g generationLifecycle[R, P]) ChangesGeneration(patch any) (bool, error) {
	p, err := generationResource[P](patch)
	if err != nil {
		return false, err
	}

	return g.policy.ChangesGeneration(p), nil
}

func (g generationLifecycle[R, P]) Select(
	desired, selected any,
	changesGeneration,
	hasEditable bool,
) (meta.GenerationSelection, error) {
	d, err := generationResource[R](desired)
	if err != nil {
		return meta.GenerationSelection{}, err
	}

	s, err := generationResource[R](selected)
	if err != nil {
		return meta.GenerationSelection{}, err
	}

	return g.policy.Select(d, s, changesGeneration, hasEditable)
}

func (g generationLifecycle[R, P]) Finalize(
	desired, current, patch any,
	selectionContext any,
	explicit bool,
) (any, error) {
	d, err := generationResource[R](desired)
	if err != nil {
		return nil, err
	}

	c, err := generationResource[R](current)
	if err != nil {
		return nil, err
	}

	p, err := generationResource[P](patch)
	if err != nil {
		return nil, err
	}

	return g.policy.Finalize(d, c, p, selectionContext, explicit)
}
