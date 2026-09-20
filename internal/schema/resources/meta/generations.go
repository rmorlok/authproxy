package meta

// GenerationState classifies observed lifecycle state without prescribing the
// names a resource uses in its wire contract.
type GenerationState uint8

const (
	GenerationEditable GenerationState = iota
	GenerationPublished
	GenerationHistorical
)

// GenerationSource identifies which server snapshot a logical update should
// reconcile against. Explicit generation requests bypass this selection.
type GenerationSource uint8

const (
	GenerationSelected GenerationSource = iota
	GenerationEditableSource
	GenerationNewest
)

// GenerationSelection carries the chosen source and immutable, policy-owned
// context needed to finalize a patch. Callers must not interpret Context.
type GenerationSelection struct {
	Source  GenerationSource
	Context any
}

// GenerationPolicy describes a resource's generation semantics using its typed
// resource and patch contracts. Functions are pure: they do not perform I/O or
// mutate their arguments. This is runtime capability metadata, not a wire schema.
//
// State classifies a stored generation, rejecting absent/unknown observed state.
// ChangesGeneration distinguishes generation changes from logical metadata edits.
// Select chooses a source using desired/selected resources, whether reconciliation
// would change the selected generation, and whether an editable generation exists.
// Finalize preserves lifecycle intent and validates explicit-generation writes;
// it receives the context from Select (nil for explicit or ordinary selected
// targets). It returns a patch without modifying inputs; callers must validate
// the result again.
type GenerationPolicy[R, P any] struct {
	State             func(*R) (GenerationState, error)
	ChangesGeneration func(*P) bool
	Select            func(desired, selected *R, changesGeneration, hasEditable bool) (GenerationSelection, error)
	Finalize          func(desired, current *R, patch *P, selectionContext any, explicit bool) (*P, error)
}
