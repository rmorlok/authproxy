package connectors

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectorJavascriptValidation preserves helper and predicate semantics
// while returning field-specific errors without authored diagnostics.
func TestConnectorJavascriptValidation(t *testing.T) {
	t.Run("helpers are available to connector predicates", func(t *testing.T) {
		// Helpers observe empty validation inputs, with statement-list and wrapper results supported.
		connector := ConnectorDefinition{
			Javascript: `
				function isEnabled(cfg) {
					return cfg.enabled === true;
				}

				function isProd(labels) {
					return labels.env === "prod";
				}

				function hasAdvancedSetup(annotations) {
					return annotations["setup-mode"] === "advanced";
				}
			`,
			Auth: testOAuth2AuthWithScopes([]Scope{
				{
					Id:       "read",
					If:       &common.Predicate{Javascript: `var enabled = isEnabled(cfg); new Boolean(enabled);`},
					Required: NewScopeRequiredPredicate(&common.Predicate{Javascript: `new Boolean(isProd(labels));`}),
					Reason:   "Read access",
				},
			}),
			SetupFlow: &SetupFlow{
				Configure: &SetupFlowPhase{
					Steps: []SetupFlowStep{
						{
							Id:         "advanced",
							If:         &common.Predicate{Javascript: `hasAdvancedSetup(annotations)`},
							JsonSchema: common.RawJSON(`{"type":"object","properties":{"enabled":{"type":"boolean"}}}`),
						},
					},
				},
			},
			Probes: []Probe{
				{
					Id:   "enabled",
					If:   &common.Predicate{Javascript: `isEnabled(cfg)`},
					Http: &ProbeHttp{Method: "GET", URL: "https://example.com/health"},
				},
			},
		}

		// A false result is valid at definition time; validation checks its type, not eligibility.
		require.NoError(t, connector.Validate(&common.ValidationContext{}))
	})

	t.Run("top level javascript must compile and run", func(t *testing.T) {
		// The submitted throw must fail validation without exposing its private value.
		connector := ConnectorDefinition{
			Javascript: `throw new Error("boom")`,
			Auth:       &Auth{InnerVal: &AuthNoAuth{Type: AuthTypeNoAuth}},
		}

		err := connector.Validate(&common.ValidationContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "javascript: invalid connector javascript")
		assert.Contains(t, err.Error(), jsruntime.ErrEvaluation.Error())
		assert.NotContains(t, err.Error(), "boom")
	})

	t.Run("helpers are unavailable when connector javascript is absent", func(t *testing.T) {
		// An unknown helper still fails the same setup field, with a safe runtime category.
		connector := ConnectorDefinition{
			Auth: &Auth{InnerVal: &AuthNoAuth{Type: AuthTypeNoAuth}},
			SetupFlow: &SetupFlow{
				Configure: &SetupFlowPhase{
					Steps: []SetupFlowStep{
						{
							Id:         "advanced",
							If:         &common.Predicate{Javascript: `isEnabled(cfg)`},
							JsonSchema: common.RawJSON(`{"type":"object","properties":{"enabled":{"type":"boolean"}}}`),
						},
					},
				},
			},
		}

		err := connector.Validate(&common.ValidationContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "setup_flow.configure.steps[0].if.javascript")
		assert.Contains(t, err.Error(), jsruntime.ErrEvaluation.Error())
		assert.NotContains(t, err.Error(), "isEnabled")
	})

	t.Run("reserved runtime variable declarations are rejected", func(t *testing.T) {
		// Connector libraries cannot replace the runtime's metadata bindings.
		connector := ConnectorDefinition{
			Javascript: `const cfg = {}`,
			Auth:       &Auth{InnerVal: &AuthNoAuth{Type: AuthTypeNoAuth}},
		}

		err := connector.Validate(&common.ValidationContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "javascript: invalid connector javascript")
		assert.Contains(t, err.Error(), jsruntime.ErrInvalidSource.Error())
		assert.Contains(t, err.Error(), `reserved runtime variable "cfg"`)
	})

	t.Run("library syntax errors retain position", func(t *testing.T) {
		// Definition authors get a useful location without parser text or source.
		connector := ConnectorDefinition{Javascript: "// privateSource\nconst x = ;"}
		err := connector.Validate(&common.ValidationContext{Path: "definition"})
		require.ErrorContains(t, err, "definition.javascript: invalid connector javascript")
		require.ErrorContains(t, err, "invalid JavaScript source at line 2, column 11")
		require.NotContains(t, err.Error(), "privateSource")
		require.NotContains(t, err.Error(), "Unexpected token")
	})
}

// TestConnectorJavascriptValidationLibraryFailures checks the bounded library
// adapter through the schema path, including the contextless validation fallback.
func TestConnectorJavascriptValidationLibraryFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		source   string
		deadline bool
		canceled bool
		want     error
	}{
		{name: "source bytes", source: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes+1), want: jsruntime.ErrLimitExceeded},
		{name: "initialization timeout", source: "for (;;) {}", want: jsruntime.ErrLimitExceeded},
		{name: "invalid source", source: "const privateSyntax = ;", want: jsruntime.ErrInvalidSource},
		{name: "runtime variables absent", source: "const privateSelection = cfg.enabled;", want: jsruntime.ErrEvaluation},
		{name: "reserved accessor throw", source: `Object.defineProperty(globalThis, "cfg", {get() {throw new Error("privateAccessor");}});`, want: jsruntime.ErrEvaluation},
		{name: "reserved accessor loop", source: `Object.defineProperty(globalThis, "cfg", {get() {for (;;) {}}});`, deadline: true, want: context.DeadlineExceeded},
		{name: "canceled before compile", source: "const privateSyntax = ;", canceled: true, want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The default context is intentionally omitted for finite synchronous validation.
			vc := &common.ValidationContext{Path: "definition"}
			if tc.deadline {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				vc.Ctx = ctx
			}
			if tc.canceled {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				vc.Ctx = ctx
			}
			connector := ConnectorDefinition{Javascript: tc.source}

			err := connector.Validate(vc)

			// ValidationError stores text rather than wrapping the underlying runtime cause.
			require.Error(t, err)
			assert.Contains(t, err.Error(), "definition.javascript: invalid connector javascript")
			assert.Contains(t, err.Error(), tc.want.Error())
			assert.NotContains(t, err.Error(), "private")
		})
	}
}

// TestConnectorJavascriptValidationCombinedSource counts the original library
// source with each predicate even when the library contains only whitespace.
func TestConnectorJavascriptValidationCombinedSource(t *testing.T) {
	// The library alone fits, but appending any condition exceeds the shared allowance.
	connector := ConnectorDefinition{
		Javascript: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes),
		Probes: []Probe{{
			Id: "health", If: &common.Predicate{Javascript: "true"},
			Http: &ProbeHttp{Method: "GET", URL: "https://example.com/health"},
		}},
	}

	err := connector.Validate(&common.ValidationContext{Path: "definition"})

	// Source overflow belongs to the predicate, not to standalone library validation.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition.probes[0].if.javascript")
	assert.Contains(t, err.Error(), jsruntime.ErrLimitExceeded.Error())
	assert.NotContains(t, err.Error(), "invalid connector javascript")
}

// TestConnectorJavascriptValidationPredicateFailures covers every schema
// predicate path so nested field/index traversal cannot discard cancellation.
func TestConnectorJavascriptValidationPredicateFailures(t *testing.T) {
	locations := map[string]func(*ConnectorDefinition) *common.Predicate{
		"auth.scopes[0].if.javascript": func(c *ConnectorDefinition) *common.Predicate {
			return c.Auth.Inner().(*AuthOAuth2).Scopes[0].If
		},
		"auth.scopes[0].required.javascript": func(c *ConnectorDefinition) *common.Predicate {
			return c.Auth.Inner().(*AuthOAuth2).Scopes[0].Required.Predicate
		},
		"probes[0].if.javascript": func(c *ConnectorDefinition) *common.Predicate { return c.Probes[0].If },
		"setup_flow.preconnect.steps[0].if.javascript": func(c *ConnectorDefinition) *common.Predicate {
			return c.SetupFlow.Preconnect.Steps[0].If
		},
		"setup_flow.configure.steps[0].if.javascript": func(c *ConnectorDefinition) *common.Predicate {
			return c.SetupFlow.Configure.Steps[0].If
		},
	}
	for location, predicate := range locations {
		t.Run(location, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				source   string
				deadline bool
				want     error
			}{
				{name: "private exception", source: `throw new Error("privateCondition");`, want: jsruntime.ErrEvaluation},
				{name: "non boolean", source: `({privateCondition: true});`, want: jsruntime.ErrInvalidOutput},
				{name: "source bytes", source: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes) + "true", want: jsruntime.ErrLimitExceeded},
				{name: "caller deadline", source: "for (;;) {}", deadline: true, want: context.DeadlineExceeded},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Keep all other fields valid to exercise this predicate's actual evaluation.
					connector := testConnectorWithJavascriptPredicates()
					predicate(&connector).Javascript = tc.source
					vc := &common.ValidationContext{Path: "definition"}
					if tc.deadline {
						ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
						defer cancel()
						vc.Ctx = ctx
					}

					err := connector.Validate(vc)

					// Every call site keeps its field path and the bounded evaluator's safe category.
					require.Error(t, err)
					assert.Contains(t, err.Error(), "definition."+location)
					assert.Contains(t, err.Error(), tc.want.Error())
					assert.NotContains(t, err.Error(), "privateCondition")
				})
			}
		})
	}
}

// TestConnectorJavascriptValidationAggregatesFailures retains independent
// schema failures even when JavaScript initialization cannot succeed.
func TestConnectorJavascriptValidationAggregatesFailures(t *testing.T) {
	// A bad library must not prevent validation of unrelated connector fields.
	connector := testConnectorWithJavascriptPredicates()
	connector.Javascript = "const privateSyntax = ;"
	connector.Probes[0].If.Javascript = "123"
	connector.SetupFlow.Configure.Steps[0].Id = ""

	err := connector.Validate(&common.ValidationContext{Path: "definition"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition.javascript")
	assert.Contains(t, err.Error(), "definition.probes[0].if.javascript")
	assert.Contains(t, err.Error(), "definition.setup_flow.configure.steps[0].id")
	assert.NotContains(t, err.Error(), "privateSyntax")
}

// testConnectorWithJavascriptPredicates builds a valid connector with each
// predicate location available for independent validation scenarios.
func testConnectorWithJavascriptPredicates() ConnectorDefinition {
	return ConnectorDefinition{
		Auth: testOAuth2AuthWithScopes([]Scope{{
			Id:       "read",
			If:       &common.Predicate{Javascript: "true"},
			Required: NewScopeRequiredPredicate(&common.Predicate{Javascript: "true"}),
		}}),
		Probes: []Probe{{
			Id: "health", If: &common.Predicate{Javascript: "true"},
			Http: &ProbeHttp{Method: "GET", URL: "https://example.com/health"},
		}},
		SetupFlow: &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id: "preconnect", If: &common.Predicate{Javascript: "true"},
				JsonSchema: common.RawJSON(`{"type":"object"}`),
			}}},
			Configure: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id: "configure", If: &common.Predicate{Javascript: "true"},
				JsonSchema: common.RawJSON(`{"type":"object"}`),
			}}},
		},
	}
}

// testOAuth2AuthWithScopes supplies the static OAuth settings needed to validate scopes.
func testOAuth2AuthWithScopes(scopes []Scope) *Auth {
	return &Auth{
		InnerVal: &AuthOAuth2{
			Type: AuthTypeOAuth2,
			ClientId: &common.StringValue{InnerVal: &common.StringValueDirect{
				Value: "client-id",
			}},
			ClientSecret: &common.StringValue{InnerVal: &common.StringValueDirect{
				Value: "client-secret",
			}},
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://example.com/oauth/authorize",
			},
			Token: AuthOauth2Token{
				Endpoint: "https://example.com/oauth/token",
			},
			Scopes: scopes,
		},
	}
}
