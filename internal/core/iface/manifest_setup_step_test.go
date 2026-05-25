package iface_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/core/iface"
)

// TestFormStep_DispatchSurface verifies that NewFormStep produces a step
// whose Type is form, whose OnSubmit invokes the supplied closure, and whose
// RenderRedirect returns the sentinel for "wrong method." This is the core
// dispatch contract callers rely on.
func TestFormStep_DispatchSurface(t *testing.T) {
	var captured json.RawMessage
	step := iface.NewFormStep(iface.FormStepConfig{
		Id:          "preconnect-0",
		Title:       "Subdomain",
		Description: "Enter your subdomain",
		JsonSchema:  json.RawMessage(`{"type":"object"}`),
		UiSchema:    json.RawMessage(`{"type":"VerticalLayout"}`),
		OnSubmit: func(_ context.Context, data json.RawMessage) error {
			captured = data
			return nil
		},
	})

	assert.Equal(t, "preconnect-0", step.Id())
	assert.Equal(t, "Subdomain", step.Title())
	assert.Equal(t, "Enter your subdomain", step.Description())
	assert.Equal(t, iface.ManifestStepTypeForm, step.Type())
	assert.JSONEq(t, `{"type":"object"}`, string(step.JsonSchema()))
	assert.JSONEq(t, `{"type":"VerticalLayout"}`, string(step.UiSchema()))

	payload := json.RawMessage(`{"subdomain":"acme"}`)
	require.NoError(t, step.OnSubmit(context.Background(), payload))
	assert.JSONEq(t, `{"subdomain":"acme"}`, string(captured))

	_, err := step.RenderRedirect(context.Background(), iface.RenderRedirectOptions{})
	assert.ErrorIs(t, err, iface.ErrRedirectNotSupported)
}

// TestFormStep_NilOnSubmitReturnsSentinel — a form step constructed without
// an OnSubmit closure surfaces ErrSubmitNotSupported on call. Lets callers
// distinguish "this step has no handler" from "the handler returned an error."
func TestFormStep_NilOnSubmitReturnsSentinel(t *testing.T) {
	step := iface.NewFormStep(iface.FormStepConfig{Id: "x"})
	err := step.OnSubmit(context.Background(), json.RawMessage(`{}`))
	assert.ErrorIs(t, err, iface.ErrSubmitNotSupported)
}

// TestFormStep_OnSubmitErrorPropagates — caller-side errors round-trip
// unchanged (not wrapped in ErrSubmitNotSupported). Important so schema
// validation failures and credential-persistence failures stay distinguishable.
func TestFormStep_OnSubmitErrorPropagates(t *testing.T) {
	sentinel := errors.New("schema validation failed: missing required field 'subdomain'")
	step := iface.NewFormStep(iface.FormStepConfig{
		Id: "x",
		OnSubmit: func(_ context.Context, _ json.RawMessage) error {
			return sentinel
		},
	})
	err := step.OnSubmit(context.Background(), nil)
	assert.ErrorIs(t, err, sentinel)
}

// TestRedirectStep_DispatchSurface verifies that NewRedirectStep produces a
// step whose Type is redirect, whose RenderRedirect invokes the supplied
// closure with ReturnToUrl threaded through, and whose OnSubmit returns the
// sentinel.
func TestRedirectStep_DispatchSurface(t *testing.T) {
	var capturedOpts iface.RenderRedirectOptions
	step := iface.NewRedirectStep(iface.RedirectStepConfig{
		Id:          "apxy:auth:oauth2_authorize",
		Title:       "Authorize",
		Description: "Sign in with your provider",
		Render: func(_ context.Context, opts iface.RenderRedirectOptions) (iface.RedirectInfo, error) {
			capturedOpts = opts
			return iface.RedirectInfo{URL: "https://provider.example.com/authorize?state=abc"}, nil
		},
	})

	assert.Equal(t, "apxy:auth:oauth2_authorize", step.Id())
	assert.Equal(t, iface.ManifestStepTypeRedirect, step.Type())
	assert.Nil(t, step.JsonSchema())
	assert.Nil(t, step.UiSchema())

	info, err := step.RenderRedirect(context.Background(), iface.RenderRedirectOptions{
		ReturnToUrl: "https://marketplace.example.com/connections",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example.com/authorize?state=abc", info.URL)
	assert.Equal(t, "https://marketplace.example.com/connections", capturedOpts.ReturnToUrl)

	assert.ErrorIs(t, step.OnSubmit(context.Background(), nil), iface.ErrSubmitNotSupported)
}

// TestRedirectStep_NilRenderReturnsSentinel — a redirect step constructed
// without a Render closure surfaces ErrRedirectNotSupported. Matches the
// form-step nil-handler behavior.
func TestRedirectStep_NilRenderReturnsSentinel(t *testing.T) {
	step := iface.NewRedirectStep(iface.RedirectStepConfig{Id: "x"})
	_, err := step.RenderRedirect(context.Background(), iface.RenderRedirectOptions{})
	assert.ErrorIs(t, err, iface.ErrRedirectNotSupported)
}
