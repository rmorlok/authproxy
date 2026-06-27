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
		steps:            steps,
		hasProbes:        connector.HasProbes(),
		hasEnabledProbes: cHasEnabledProbes(c),
		authBoundary:     authBoundary,
	}
}

func cHasEnabledProbes(c iface.Connection) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		probes, err := c.GetEnabledProbes(ctx)
		if err != nil {
			return false, err
		}
		return len(probes) > 0, nil
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
		IsEligible:  newSchemaStepEligibility(c, spec),
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
// at render time: {{RETURN_ADVANCE}} → /setup/connections/{id}/advance?token=...
// and {{RETURN_ABORT}} → the matching /abort URL. Each placeholder is
// expanded by minting a fresh one-time-use setup_token bound to this
// connection + step + the caller's marketplace return URL.
func (s *service) newSchemaRedirectStep(c iface.Connection, spec *cschema.SetupFlowStep) iface.ManifestSetupStep {
	return iface.NewRedirectStep(iface.RedirectStepConfig{
		Id:          spec.Id,
		Title:       spec.Title,
		Description: spec.Description,
		IsEligible:  newSchemaStepEligibility(c, spec),
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

func newSchemaStepEligibility(c iface.Connection, spec *cschema.SetupFlowStep) func(context.Context) (bool, error) {
	if spec.If == nil {
		return nil
	}
	return func(ctx context.Context) (bool, error) {
		jsCtx, err := c.GetJavascriptContext(ctx)
		if err != nil {
			return false, fmt.Errorf("step %q: get javascript context: %w", spec.Id, err)
		}

		ok, err := spec.If.GetValue(jsCtx)
		if err != nil {
			return false, fmt.Errorf("step %q if.javascript: %w", spec.Id, err)
		}
		return ok, nil
	}
}

// mintReturnURL mints a setup_token and returns the public-endpoint URL
// (/setup/connections/{id}/{advance|abort}?token=<jti>) that substitutes
// for the corresponding placeholder in a redirect step's URL template.
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
	return fmt.Sprintf("%s/setup/connections/%s/%s?token=%s",
		strings.TrimRight(publicBase, "/"),
		connectionId,
		endpoint,
		tok,
	), nil
}

// manifestFlow is the materialized setup flow for a single connection.
// Exposed as iface.ManifestSetupFlow.
type manifestFlow struct {
	steps            []iface.ManifestSetupStep
	hasProbes        bool
	hasEnabledProbes func(context.Context) (bool, error)
	authBoundary     int // index in steps after which verify slots in when probes are enabled
}

func (f *manifestFlow) Steps(ctx context.Context) ([]iface.ManifestSetupStep, error) {
	out := make([]iface.ManifestSetupStep, 0, len(f.steps))
	for i := range f.steps {
		eligible, err := f.isStepEligible(ctx, i)
		if err != nil {
			return nil, err
		}
		if eligible {
			out = append(out, f.steps[i])
		}
	}
	return out, nil
}

func (f *manifestFlow) StepById(ctx context.Context, id string) (iface.ManifestSetupStep, bool, error) {
	if id == "" {
		return nil, false, nil
	}
	if id == iface.NewVerifyStep().Id() && f.hasProbes {
		ok, err := f.verifyEnabled(ctx)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		return iface.NewVerifyStep(), true, nil
	}
	for i, s := range f.steps {
		if s.Id() != id {
			continue
		}
		eligible, err := f.isStepEligible(ctx, i)
		if err != nil {
			return nil, false, err
		}
		return s, eligible, nil
	}
	return nil, false, nil
}

func (f *manifestFlow) ContainsStep(id string) bool {
	if id == "" {
		return false
	}
	if id == iface.NewVerifyStep().Id() && f.hasProbes {
		return true
	}
	for _, step := range f.steps {
		if step.Id() == id {
			return true
		}
	}
	return false
}

func (f *manifestFlow) FirstStep(ctx context.Context) (iface.ManifestSetupStep, error) {
	if f.hasProbes && f.authBoundary >= 0 {
		step, ok, err := f.nextEligibleStep(ctx, 0, f.authBoundary)
		if err != nil || ok {
			return step, err
		}
		verify, err := f.verifyEnabled(ctx)
		if err != nil {
			return nil, err
		}
		if verify {
			return iface.NewVerifyStep(), nil
		}
	}
	step, _, err := f.nextEligibleStep(ctx, 0, len(f.steps)-1)
	return step, err
}

func (f *manifestFlow) NextStep(ctx context.Context, currentId string) (iface.ManifestSetupStep, bool, error) {
	verifyId := iface.NewVerifyStep().Id()

	// Coming out of verify: jump to the step right after the auth boundary
	// (the first configure step, typically).
	if currentId == verifyId {
		return f.nextEligibleStep(ctx, f.authBoundary+1, len(f.steps)-1)
	}

	for i, step := range f.steps {
		if step.Id() != currentId {
			continue
		}
		// Verify insertion: when the current step is the last one in the
		// credential-establishing phase, transition to verify next. Gated-off
		// steps in the same phase are skipped before verify is considered.
		if f.hasProbes && i <= f.authBoundary {
			next, ok, err := f.nextEligibleStep(ctx, i+1, f.authBoundary)
			if err != nil || ok {
				return next, ok, err
			}
			verify, err := f.verifyEnabled(ctx)
			if err != nil {
				return nil, false, err
			}
			if verify {
				return iface.NewVerifyStep(), true, nil
			}
			return f.nextEligibleStep(ctx, f.authBoundary+1, len(f.steps)-1)
		}
		return f.nextEligibleStep(ctx, i+1, len(f.steps)-1)
	}
	return nil, false, nil
}

func (f *manifestFlow) verifyEnabled(ctx context.Context) (bool, error) {
	if !f.hasProbes {
		return false, nil
	}
	if f.hasEnabledProbes == nil {
		return true, nil
	}
	return f.hasEnabledProbes(ctx)
}

func (f *manifestFlow) nextEligibleStep(ctx context.Context, start int, end int) (iface.ManifestSetupStep, bool, error) {
	if start < 0 {
		start = 0
	}
	if end >= len(f.steps) {
		end = len(f.steps) - 1
	}
	if start > end {
		return nil, false, nil
	}
	for i := start; i <= end; i++ {
		eligible, err := f.isStepEligible(ctx, i)
		if err != nil {
			return nil, false, err
		}
		if eligible {
			return f.steps[i], true, nil
		}
	}
	return nil, false, nil
}

func (f *manifestFlow) isStepEligible(ctx context.Context, idx int) (bool, error) {
	if idx < 0 || idx >= len(f.steps) {
		return false, nil
	}
	return f.steps[idx].IsEligible(ctx)
}
