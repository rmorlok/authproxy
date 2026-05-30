package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aptmpl"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/core/setup_token"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// setupTokenTTL is how long RETURN_ADVANCE / RETURN_ABORT tokens stay
// usable after a redirect step is rendered. Long enough to survive a user
// finishing a 3rd-party off-platform step (often minutes of clicking
// through provider flows); short enough that a leaked token expires
// before it can be abused.
const setupTokenTTL = 15 * time.Minute

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
			steps = append(steps, s.newSchemaStep(c, &connector.SetupFlow.Preconnect.Steps[i]))
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
			steps = append(steps, s.newSchemaStep(c, &connector.SetupFlow.Configure.Steps[i]))
		}
	}

	return &manifestFlow{
		steps:         steps,
		hasProbes:     connector.HasProbes(),
		authBoundary:  authBoundary,
	}
}

// newSchemaStep wraps a connector-YAML SetupFlowStep into the appropriate
// ManifestSetupStep — a form step (merges submitted fields into
// EncryptedConfiguration on submit) or a redirect step (renders the URL
// template with cfg mustache + freshly-minted RETURN_ADVANCE / RETURN_ABORT
// tokens).
func (s *service) newSchemaStep(c iface.Connection, spec *cschema.SetupFlowStep) iface.ManifestSetupStep {
	if spec.Type.Normalized() == cschema.SetupFlowStepTypeRedirect {
		return s.newSchemaRedirectStep(c, spec)
	}
	return newSchemaFormStep(c, spec)
}

// newSchemaFormStep wraps a form-kind SetupFlowStep. spec is captured by
// closure so the OnSubmit closure can re-validate the data on each submit.
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

// newSchemaRedirectStep wraps a redirect-kind SetupFlowStep. The closure
// substitutes the connection's cfg mustache plus two synthetic placeholders
// at render time: {{RETURN_ADVANCE}} → /public/connections/{id}/setup/advance?token=...
// and {{RETURN_ABORT}} → the matching /abort URL. Each placeholder is
// expanded by minting a fresh one-time-use setup_token bound to this
// connection + step + the caller's marketplace return URL.
func (s *service) newSchemaRedirectStep(c iface.Connection, spec *cschema.SetupFlowStep) iface.ManifestSetupStep {
	return iface.NewRedirectStep(iface.RedirectStepConfig{
		Id:          spec.Id,
		Title:       spec.Title,
		Description: spec.Description,
		Render: func(ctx context.Context, opts iface.RenderRedirectOptions) (iface.RedirectInfo, error) {
			if spec.Redirect == nil || spec.Redirect.URL == "" {
				return iface.RedirectInfo{}, fmt.Errorf("redirect step %q has no URL configured", spec.Id)
			}

			// 1) cfg mustache: render the URL template against the
			//    connection's mustache context first so cfg-derived
			//    placeholders are resolved before we inject the synthetic
			//    return-tokens.
			mctx, err := c.GetMustacheContext(ctx)
			if err != nil {
				return iface.RedirectInfo{}, fmt.Errorf("redirect step %q: get mustache context: %w", spec.Id, err)
			}
			rendered, err := aptmpl.RenderMustache(spec.Redirect.URL, mctx)
			if err != nil {
				return iface.RedirectInfo{}, fmt.Errorf("redirect step %q: render template: %w", spec.Id, err)
			}

			// 2) RETURN_ADVANCE / RETURN_ABORT: mint fresh one-time-use
			//    tokens bound to the initiating actor, and substitute
			//    the resulting public-endpoint URLs.
			ra := apauthcore.GetAuthFromContext(ctx)
			actor := ra.MustGetActor()
			advanceURL, err := s.mintReturnURL(ctx, c.GetId(), spec.Id, actor.GetId(), opts.ReturnToUrl, setup_token.IntentAdvance)
			if err != nil {
				return iface.RedirectInfo{}, fmt.Errorf("redirect step %q: mint advance token: %w", spec.Id, err)
			}
			abortURL, err := s.mintReturnURL(ctx, c.GetId(), spec.Id, actor.GetId(), opts.ReturnToUrl, setup_token.IntentAbort)
			if err != nil {
				return iface.RedirectInfo{}, fmt.Errorf("redirect step %q: mint abort token: %w", spec.Id, err)
			}

			rendered = strings.ReplaceAll(rendered, "{{RETURN_ADVANCE}}", advanceURL)
			rendered = strings.ReplaceAll(rendered, "{{RETURN_ABORT}}", abortURL)

			return iface.RedirectInfo{URL: rendered}, nil
		},
	})
}

// mintReturnURL mints a setup_token and returns the public-endpoint URL
// (/public/connections/{id}/setup/{advance|abort}?token=<jti>) that
// substitutes for the corresponding placeholder in a redirect step's
// URL template.
func (s *service) mintReturnURL(ctx context.Context, connectionId apid.ID, stepId string, actorId apid.ID, returnToUrl string, intent setup_token.Intent) (string, error) {
	tok, err := setup_token.Mint(ctx, s.r, s.encrypt, setup_token.MintInput{
		ConnectionId: connectionId,
		StepId:       stepId,
		ActorId:      actorId,
		Intent:       intent,
		ReturnToUrl:  returnToUrl,
	}, setupTokenTTL)
	if err != nil {
		return "", err
	}

	publicBase := s.cfg.GetRoot().Public.GetBaseUrl()
	endpoint := "advance"
	if intent == setup_token.IntentAbort {
		endpoint = "abort"
	}
	// Token rides as a query parameter; jti is opaque (apid.ID-shaped) and
	// URL-safe by construction so we can embed verbatim.
	return fmt.Sprintf("%s/public/connections/%s/setup/%s?token=%s",
		strings.TrimRight(publicBase, "/"),
		connectionId,
		endpoint,
		tok,
	), nil
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
