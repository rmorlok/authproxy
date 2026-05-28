package core

import (
	"context"
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// buildManifestSetupFlow assembles the ManifestSetupFlow for a connection.
// The linear order is preconnect (schema) + auth-method-emitted steps
// (factory) + configure (schema). When the connector has probes, the
// apxy:verify pseudo-step is inserted between the last credential-
// establishing step and the first configure step (or returned as the next
// step from the last credential-establishing step when there are no
// configure steps).
//
// Schema-defined form steps carry an OnSubmit closure that validates against
// the step's JSON Schema and merges allowed fields into the connection's
// EncryptedConfiguration. Auth-method steps are constructed by the auth
// method's factory and carry their own OnSubmit / RenderRedirect closures.
func (s *service) buildManifestSetupFlow(c iface.Connection) iface.ManifestSetupFlow {
	cv := c.GetConnectorVersionEntity()
	if cv == nil {
		return &manifestFlow{}
	}
	connector := cv.GetDefinition()
	if connector == nil {
		return &manifestFlow{}
	}

	var steps []iface.ManifestSetupStep
	authBoundary := -1 // index in steps after which verify is inserted

	if connector.SetupFlow != nil && connector.SetupFlow.HasPreconnect() {
		for i := range connector.SetupFlow.Preconnect.Steps {
			steps = append(steps, newSchemaFormStep(c, &connector.SetupFlow.Preconnect.Steps[i]))
		}
	}

	if factory := s.getAuthMethodFactory(connector); factory != nil {
		authSteps := factory.ManifestSetupSteps(c, connector)
		steps = append(steps, authSteps...)
	}

	// Verify is inserted after the last step of the credential-establishing
	// phase — which is the last step in `steps` *before* configure is
	// appended. If there are no preconnect/auth steps either, verify
	// effectively becomes the first step (degenerate case).
	authBoundary = len(steps) - 1

	if connector.SetupFlow != nil && connector.SetupFlow.HasConfigure() {
		for i := range connector.SetupFlow.Configure.Steps {
			steps = append(steps, newSchemaFormStep(c, &connector.SetupFlow.Configure.Steps[i]))
		}
	}

	return &manifestFlow{
		steps:         steps,
		hasProbes:     connector.HasProbes(),
		authBoundary:  authBoundary,
	}
}

// newSchemaFormStep wraps a connector-YAML SetupFlowStep into a
// ManifestSetupStep whose OnSubmit merges submitted fields into the
// connection's EncryptedConfiguration. spec is captured by closure; tests
// that inspect the produced step compare its JsonSchema / UiSchema against
// the spec.
func newSchemaFormStep(c iface.Connection, spec *cschema.SetupFlowStep) iface.ManifestSetupStep {
	return iface.NewFormStep(iface.FormStepConfig{
		Id:          spec.Id,
		Title:       spec.Title,
		Description: spec.Description,
		JsonSchema:  json.RawMessage(spec.JsonSchema),
		UiSchema:    json.RawMessage(spec.UiSchema),
		OnSubmit: func(ctx context.Context, data json.RawMessage) error {
			existing, err := c.GetConfiguration(ctx)
			if err != nil {
				return httperr.InternalServerError(httperr.WithInternalErrorf("failed to get existing configuration: %w", err))
			}
			merged, err := spec.ValidateAndMergeData(spec.Id, data, existing)
			if err != nil {
				return httperr.BadRequest(err.Error())
			}
			if err := c.SetConfiguration(ctx, merged); err != nil {
				return httperr.InternalServerError(httperr.WithInternalErrorf("failed to save configuration: %w", err))
			}
			return nil
		},
	})
}

// manifestFlow is the materialized setup flow for a single connection.
// Exposed as iface.ManifestSetupFlow.
type manifestFlow struct {
	steps        []iface.ManifestSetupStep
	hasProbes    bool
	authBoundary int // index in steps after which verify slots in when hasProbes
}

func (f *manifestFlow) Steps() []iface.ManifestSetupStep {
	// Defensive copy so callers can't mutate the underlying slice.
	out := make([]iface.ManifestSetupStep, len(f.steps))
	copy(out, f.steps)
	return out
}

func (f *manifestFlow) StepById(id string) (iface.ManifestSetupStep, bool) {
	if id == "" {
		return nil, false
	}
	if id == iface.NewVerifyStep().Id() && f.hasProbes {
		return iface.NewVerifyStep(), true
	}
	for _, s := range f.steps {
		if s.Id() == id {
			return s, true
		}
	}
	return nil, false
}

func (f *manifestFlow) FirstStep() iface.ManifestSetupStep {
	if len(f.steps) == 0 {
		return nil
	}
	return f.steps[0]
}

func (f *manifestFlow) NextStep(currentId string) (iface.ManifestSetupStep, bool) {
	verifyId := iface.NewVerifyStep().Id()

	// Coming out of verify: jump to the step right after the auth boundary
	// (the first configure step, typically).
	if currentId == verifyId {
		next := f.authBoundary + 1
		if next < 0 || next >= len(f.steps) {
			return nil, false
		}
		return f.steps[next], true
	}

	for i, step := range f.steps {
		if step.Id() != currentId {
			continue
		}
		// Verify insertion: when the current step is the last one in the
		// credential-establishing phase, transition to verify next.
		if f.hasProbes && i == f.authBoundary {
			return iface.NewVerifyStep(), true
		}
		if i+1 < len(f.steps) {
			return f.steps[i+1], true
		}
		return nil, false
	}
	return nil, false
}
