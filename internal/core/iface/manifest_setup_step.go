package iface

import (
	"context"
	"encoding/json"
	"errors"
)

// ManifestStepType identifies the runtime kind of a setup step. The schema
// distinguishes user-authored kinds (form vs. redirect) — those carry through
// to the manifest unchanged, plus the auth-method-emitted steps that get one
// or the other depending on what the method needs at that point in the flow
// (api-key emits form steps for credential collection; OAuth2 emits a redirect
// step for the authorize URL).
type ManifestStepType string

const (
	ManifestStepTypeForm     ManifestStepType = "form"
	ManifestStepTypeRedirect ManifestStepType = "redirect"
	// ManifestStepTypeVerify is the type tag for the synthetic apxy:verify
	// pseudo-step: the connection is waiting for the verify task to finish.
	// The user sees a "verifying" response and polls; OnSubmit /
	// RenderRedirect both return the matching sentinel errors.
	ManifestStepTypeVerify ManifestStepType = "verify"
)

// Sentinel errors returned when a method is invoked on a step that doesn't
// support it (e.g. calling OnSubmit on a redirect step). Callers should
// dispatch by Type() before invoking the method; the sentinels exist so that
// programmer errors fail loudly instead of silently misbehaving.
var (
	ErrSubmitNotSupported   = errors.New("step does not accept form submissions")
	ErrRedirectNotSupported = errors.New("step is not a redirect step")
)

// RedirectInfo is the resolved redirect for a redirect-type step. The setup
// flow returns this from RenderRedirect; the HTTP layer turns it into the
// ConnectionSetupRedirect response. Single-field today; expandable when a
// future step kind needs e.g. POST + body or additional headers.
type RedirectInfo struct {
	// URL is the fully-rendered URL the user should be redirected to. For
	// schema-defined redirect steps this is the URL template with cfg + token
	// substitution applied; for OAuth2-emitted redirect steps this is the
	// freshly-minted authorize URL (state + PKCE already persisted).
	URL string
}

// RenderRedirectOptions carries request-scoped inputs into a redirect step's
// URL resolution. Threaded through the flow runner because the values are
// known to the HTTP layer but not the step itself.
type RenderRedirectOptions struct {
	// ReturnToUrl is the marketplace URL the 3rd party should bounce the user
	// back to after they complete the off-platform step. Empty when the
	// caller does not have one — schema-defined redirect steps that use
	// RETURN_ADVANCE/RETURN_ABORT tokens (see #367) read ReturnToUrl to
	// embed in the signed token's claim payload; OAuth2 reads it to persist
	// in the state record.
	ReturnToUrl string
}

// ManifestSetupStep is a single, fully-resolved step in a connection's setup
// flow. Owners (schema-defined steps from the connector YAML, auth-method-
// emitted steps from the auth method's factory) construct ManifestSetupStep
// values; core's flow runner dispatches against them without knowing where
// they came from.
//
// All steps expose presentation metadata (Id, Title, Description, Type). Type
// determines which of OnSubmit / RenderRedirect is meaningful — calling the
// other returns the matching sentinel error.
type ManifestSetupStep interface {
	// Id is the step identifier. User-authored steps carry the id from the
	// connector YAML; system-emitted steps use the apxy: prefix (e.g.
	// "apxy:auth:oauth2_authorize", "apxy:verify_failed").
	Id() string

	// Title and Description are the human-readable strings the UI renders
	// above the form / redirect notice. Both may be empty.
	Title() string
	Description() string

	// Type selects the dispatch branch — form steps receive OnSubmit, redirect
	// steps receive RenderRedirect.
	Type() ManifestStepType

	// JsonSchema and UiSchema are the JSONForms-shaped payloads for form
	// steps. Both return nil for redirect steps.
	JsonSchema() json.RawMessage
	UiSchema() json.RawMessage

	// OnSubmit handles a form submission for this step. Implementations:
	//   - schema-defined form steps: validate against JsonSchema and merge
	//     the allowed fields into the connection's EncryptedConfiguration.
	//   - auth-method-emitted form steps (e.g. api-key credential
	//     collection): hand the data to the auth method to persist as
	//     credentials, then trigger HandleCredentialsEstablished.
	//   - redirect steps: return ErrSubmitNotSupported.
	OnSubmit(ctx context.Context, data json.RawMessage) error

	// RenderRedirect returns the resolved redirect URL for redirect-type
	// steps. Form steps return ErrRedirectNotSupported. The opts struct
	// carries request-scoped values (ReturnToUrl) the step may need to mint
	// the URL.
	RenderRedirect(ctx context.Context, opts RenderRedirectOptions) (RedirectInfo, error)
}

// ManifestSetupFlow is the ordered, fully-materialized setup flow for a
// connection. Built by core (the flow builder lands in #366) from: preconnect
// steps (schema) + auth-method-emitted steps (factory) + configure steps
// (schema). Terminal failure pseudo-steps (apxy:verify_failed,
// apxy:auth_failed) and the verify step (apxy:verify) are addressable by id
// but not part of the linear order returned by Steps() / NextStep().
type ManifestSetupFlow interface {
	// Steps returns the ordered, linear steps a user walks through during
	// setup. Implicit terminal steps (verify_failed, auth_failed) and the
	// verify pseudo-step are not included; use StepById for those.
	Steps() []ManifestSetupStep

	// StepById returns the step with the given id, including implicit
	// terminal/verify pseudo-steps. Returns false if the id is unknown.
	StepById(id string) (ManifestSetupStep, bool)

	// FirstStep returns the first step in the linear flow. Returns nil if
	// the flow has no linear steps (a connector with no preconnect, no
	// auth-emitted steps, and no configure — which means setup is already
	// complete on initiate).
	FirstStep() ManifestSetupStep

	// NextStep returns the step that follows the step with the given id in
	// the linear flow, or false when currentId is the final linear step.
	// Pseudo-steps (verify, verify_failed, auth_failed) are not part of the
	// linear successor relation.
	NextStep(currentId string) (ManifestSetupStep, bool)
}

// FormStepConfig configures NewFormStep. All fields are optional except Id
// and JsonSchema; OnSubmit is required for the step to handle submissions
// (a nil submitter returns ErrSubmitNotSupported, which is correct for
// purely-display intermediate steps if those ever exist).
type FormStepConfig struct {
	Id          string
	Title       string
	Description string
	JsonSchema  json.RawMessage
	UiSchema    json.RawMessage
	OnSubmit    func(ctx context.Context, data json.RawMessage) error
}

// NewFormStep returns a form-type ManifestSetupStep backed by the supplied
// configuration. The OnSubmit closure is what differs across owners (schema
// merges into config; api-key persists credentials).
func NewFormStep(cfg FormStepConfig) ManifestSetupStep {
	return &formStep{cfg: cfg}
}

// RedirectStepConfig configures NewRedirectStep.
type RedirectStepConfig struct {
	Id          string
	Title       string
	Description string
	// Render resolves the redirect URL when the flow runner reaches this
	// step. Required.
	Render func(ctx context.Context, opts RenderRedirectOptions) (RedirectInfo, error)
}

// NewRedirectStep returns a redirect-type ManifestSetupStep backed by the
// supplied configuration.
func NewRedirectStep(cfg RedirectStepConfig) ManifestSetupStep {
	return &redirectStep{cfg: cfg}
}

// NewVerifyStep returns the synthetic apxy:verify pseudo-step that
// ManifestSetupFlow uses to mark "credentials established, probes running."
// The id and title are fixed; OnSubmit / RenderRedirect both return the
// matching sentinel.
func NewVerifyStep() ManifestSetupStep {
	return &verifyStep{}
}

type formStep struct {
	cfg FormStepConfig
}

func (s *formStep) Id() string                       { return s.cfg.Id }
func (s *formStep) Title() string                    { return s.cfg.Title }
func (s *formStep) Description() string              { return s.cfg.Description }
func (s *formStep) Type() ManifestStepType           { return ManifestStepTypeForm }
func (s *formStep) JsonSchema() json.RawMessage      { return s.cfg.JsonSchema }
func (s *formStep) UiSchema() json.RawMessage        { return s.cfg.UiSchema }

func (s *formStep) OnSubmit(ctx context.Context, data json.RawMessage) error {
	if s.cfg.OnSubmit == nil {
		return ErrSubmitNotSupported
	}
	return s.cfg.OnSubmit(ctx, data)
}

func (s *formStep) RenderRedirect(_ context.Context, _ RenderRedirectOptions) (RedirectInfo, error) {
	return RedirectInfo{}, ErrRedirectNotSupported
}

type redirectStep struct {
	cfg RedirectStepConfig
}

func (s *redirectStep) Id() string                  { return s.cfg.Id }
func (s *redirectStep) Title() string               { return s.cfg.Title }
func (s *redirectStep) Description() string         { return s.cfg.Description }
func (s *redirectStep) Type() ManifestStepType      { return ManifestStepTypeRedirect }
func (s *redirectStep) JsonSchema() json.RawMessage { return nil }
func (s *redirectStep) UiSchema() json.RawMessage   { return nil }

func (s *redirectStep) OnSubmit(_ context.Context, _ json.RawMessage) error {
	return ErrSubmitNotSupported
}

func (s *redirectStep) RenderRedirect(ctx context.Context, opts RenderRedirectOptions) (RedirectInfo, error) {
	if s.cfg.Render == nil {
		return RedirectInfo{}, ErrRedirectNotSupported
	}
	return s.cfg.Render(ctx, opts)
}

// verifyStep is the synthetic apxy:verify pseudo-step. Stateless — no
// configuration is needed since its sole purpose is to mark "probes are
// running; the UI should display Verifying."
type verifyStep struct{}

func (s *verifyStep) Id() string                  { return "apxy:verify" }
func (s *verifyStep) Title() string               { return "Verifying connection" }
func (s *verifyStep) Description() string         { return "" }
func (s *verifyStep) Type() ManifestStepType      { return ManifestStepTypeVerify }
func (s *verifyStep) JsonSchema() json.RawMessage { return nil }
func (s *verifyStep) UiSchema() json.RawMessage   { return nil }

func (s *verifyStep) OnSubmit(_ context.Context, _ json.RawMessage) error {
	return ErrSubmitNotSupported
}

func (s *verifyStep) RenderRedirect(_ context.Context, _ RenderRedirectOptions) (RedirectInfo, error) {
	return RedirectInfo{}, ErrRedirectNotSupported
}
