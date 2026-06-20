package connectors

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectorJavascriptValidation(t *testing.T) {
	t.Run("helpers are available to connector predicates", func(t *testing.T) {
		connector := Connector{
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
					If:       &common.Predicate{Javascript: `isEnabled(cfg)`},
					Required: NewScopeRequiredPredicate(&common.Predicate{Javascript: `isProd(labels)`}),
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

		require.NoError(t, connector.Validate(&common.ValidationContext{}))
	})

	t.Run("top level javascript must compile and run", func(t *testing.T) {
		connector := Connector{
			Javascript: `throw new Error("boom")`,
			Auth:       &Auth{InnerVal: &AuthNoAuth{Type: AuthTypeNoAuth}},
		}

		err := connector.Validate(&common.ValidationContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "javascript: invalid connector javascript")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("helpers are unavailable when connector javascript is absent", func(t *testing.T) {
		connector := Connector{
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
		assert.Contains(t, err.Error(), "isEnabled is not defined")
	})

	t.Run("reserved runtime variable declarations are rejected", func(t *testing.T) {
		connector := Connector{
			Javascript: `const cfg = {}`,
			Auth:       &Auth{InnerVal: &AuthNoAuth{Type: AuthTypeNoAuth}},
		}

		err := connector.Validate(&common.ValidationContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `reserved runtime variable "cfg"`)
	})
}

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
