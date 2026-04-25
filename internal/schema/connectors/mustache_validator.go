package connectors

import (
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// MustacheValidationContext carries the cfg.X field-availability data that
// validators consult when cross-checking mustache references against a
// connector's setup flow. Different lifecycle points have different scopes:
//
//   - Auth-phase templates (OAuth2 Authorization, Token) may only reference
//     PreconnectFields.
//   - Post-setup templates (OAuth2 Revocation) may reference AllConfigFields.
//   - Data sources rendered during a configure step use ConfigureStepFields,
//     which limits visibility to fields collected before that step runs.
type MustacheValidationContext struct {
	// PreconnectFields are the cfg fields populated by preconnect form steps.
	// Empty map if the connector has no preconnect phase.
	PreconnectFields map[string]bool

	// AllConfigFields contains every cfg field declared anywhere in the setup
	// flow (preconnect + every configure step). Empty map if there is no
	// setup flow.
	AllConfigFields map[string]bool

	// setupFlow backs the step-relative ConfigureStepFields query.
	setupFlow *SetupFlow
}

// NewMustacheValidationContext builds a context from a (possibly nil) setup flow,
// pre-computing the auth-phase and post-setup field sets so validators can read
// them without recomputation.
func NewMustacheValidationContext(sf *SetupFlow) *MustacheValidationContext {
	return &MustacheValidationContext{
		PreconnectFields: sf.PreconnectFieldNames(),
		AllConfigFields:  sf.AllConfigFieldNames(),
		setupFlow:        sf,
	}
}

// ConfigureStepFields returns the cfg fields available while a configure step
// at the given index is being filled out — preconnect fields plus configure
// steps with index < stepIdx.
func (m *MustacheValidationContext) ConfigureStepFields(stepIdx int) map[string]bool {
	if m == nil {
		return nil
	}
	if m.setupFlow == nil {
		return m.PreconnectFields
	}
	return util.UnionBoolMaps(m.PreconnectFields, m.setupFlow.ConfigureFieldNamesUpTo(stepIdx))
}

// MustacheValidator is implemented by a templated subtree of a connector
// definition (e.g. an Auth implementation) that wants to cross-check its
// mustache references against the connector's setup flow. The implementation
// chooses the appropriate scope on the provided context for the lifecycle
// phase in which its templates render.
type MustacheValidator interface {
	ValidateMustacheReferences(vc *common.ValidationContext, mctx *MustacheValidationContext) error
}
