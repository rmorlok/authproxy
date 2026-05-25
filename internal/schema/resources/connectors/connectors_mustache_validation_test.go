package connectors

import (
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOAuthConnector(oauth *AuthOAuth2, sf *SetupFlow) *Connector {
	return &Connector{
		Labels:      map[string]string{"type": "test"},
		DisplayName: "Test",
		Description: "Test",
		Auth:        &Auth{InnerVal: oauth},
		SetupFlow:   sf,
	}
}

func preconnectFlow(jsonSchema string) *SetupFlow {
	return &SetupFlow{
		Preconnect: &SetupFlowPhase{
			Steps: []SetupFlowStep{
				{
					Id:         "preconnect-1",
					JsonSchema: common.RawJSON(jsonSchema),
				},
			},
		},
	}
}

func configureFlow(jsonSchema string) *SetupFlow {
	return &SetupFlow{
		Configure: &SetupFlowPhase{
			Steps: []SetupFlowStep{
				{
					Id:         "configure-1",
					JsonSchema: common.RawJSON(jsonSchema),
				},
			},
		},
	}
}

func TestValidateMustacheReferences_OAuthAuthorization(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("authorization endpoint cfg ref present in preconnect", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://{{cfg.tenant}}.example.com/oauth/authorize",
			},
		}, preconnectFlow(`{"type":"object","properties":{"tenant":{"type":"string"}}}`))
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("authorization endpoint cfg ref missing entirely", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://{{cfg.tenant}}.example.com/oauth/authorize",
			},
		}, nil)
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth.authorization.endpoint")
		assert.Contains(t, err.Error(), "{{cfg.tenant}}")
		assert.Contains(t, err.Error(), "preconnect")
	})

	t.Run("authorization endpoint cfg ref defined only in configure errors", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://{{cfg.tenant}}.example.com/oauth/authorize",
			},
		}, configureFlow(`{"type":"object","properties":{"tenant":{"type":"string"}}}`))
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "{{cfg.tenant}}")
		assert.Contains(t, err.Error(), "preconnect")
	})

	t.Run("authorization query overrides validated against preconnect", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://example.com/authorize",
				QueryOverrides: map[string]string{
					"resource": "https://{{cfg.tenant}}.example.com/api",
				},
			},
		}, nil)
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth.authorization.query_overrides.resource")
	})

	t.Run("static endpoint with no setup flow is fine", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://example.com/authorize",
			},
		}, nil)
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("labels and annotations refs are not validated", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://{{labels.env}}.{{annotations.region}}.example.com/authorize",
			},
		}, nil)
		require.NoError(t, c.validateMustacheReferences(vc))
	})
}

func TestValidateMustacheReferences_OAuthToken(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("token endpoint requires preconnect cfg", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Token: AuthOauth2Token{
				Endpoint: "https://{{cfg.tenant}}.example.com/oauth/token",
			},
		}, configureFlow(`{"type":"object","properties":{"tenant":{"type":"string"}}}`))
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth.token.endpoint")
	})

	t.Run("token form overrides validated against preconnect", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Token: AuthOauth2Token{
				Endpoint: "https://example.com/token",
				FormOverrides: map[string]string{
					"audience": "{{cfg.audience}}",
				},
			},
		}, preconnectFlow(`{"type":"object","properties":{"audience":{"type":"string"}}}`))
		require.NoError(t, c.validateMustacheReferences(vc))
	})
}

func TestValidateMustacheReferences_OAuthRevocation(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("revocation accepts cfg from configure step", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Revocation: &AuthOauth2Revocation{
				Endpoint: "https://{{cfg.workspace}}.example.com/oauth/revoke",
			},
		}, configureFlow(`{"type":"object","properties":{"workspace":{"type":"string"}}}`))
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("revocation accepts cfg from preconnect", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Revocation: &AuthOauth2Revocation{
				Endpoint: "https://{{cfg.tenant}}.example.com/oauth/revoke",
			},
		}, preconnectFlow(`{"type":"object","properties":{"tenant":{"type":"string"}}}`))
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("revocation rejects unknown cfg", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Revocation: &AuthOauth2Revocation{
				Endpoint: "https://{{cfg.unknown}}.example.com/oauth/revoke",
			},
		}, preconnectFlow(`{"type":"object","properties":{"tenant":{"type":"string"}}}`))
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth.revocation.endpoint")
		assert.Contains(t, err.Error(), "{{cfg.unknown}}")
	})
}

func TestValidateMustacheReferences_DataSources(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("data source url uses preconnect field", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{Type: AuthTypeOAuth2}, &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{Id: "p1", JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`)},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "c1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"workspaces": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://{{cfg.tenant}}.example.com/workspaces",
								},
								Transform: "data.map(w => ({value: w.id, label: w.name}))",
							},
						},
					},
				},
			},
		})
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("data source can reference earlier configure step", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{Type: AuthTypeOAuth2}, &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{Id: "c1", JsonSchema: common.RawJSON(`{"type":"object","properties":{"workspace":{"type":"string"}}}`)},
					{
						Id:         "c2",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"projects": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://example.com/{{cfg.workspace}}/projects",
								},
								Transform: "data.map(p => ({value: p.id, label: p.name}))",
							},
						},
					},
				},
			},
		})
		require.NoError(t, c.validateMustacheReferences(vc))
	})

	t.Run("data source cannot reference field from same step", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{Type: AuthTypeOAuth2}, &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "c1",
						JsonSchema: common.RawJSON(`{"type":"object","properties":{"workspace":{"type":"string"}}}`),
						DataSources: map[string]DataSourceDef{
							"projects": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://example.com/{{cfg.workspace}}/projects",
								},
								Transform: "data.map(p => ({value: p.id, label: p.name}))",
							},
						},
					},
				},
			},
		})
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_sources.projects.proxy_request.url")
		assert.Contains(t, err.Error(), "{{cfg.workspace}}")
	})

	t.Run("data source headers validated", func(t *testing.T) {
		c := newOAuthConnector(&AuthOAuth2{Type: AuthTypeOAuth2}, &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "c1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"projects": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://example.com/projects",
									Headers: map[string]string{
										"X-Tenant": "{{cfg.missing}}",
									},
								},
								Transform: "data",
							},
						},
					},
				},
			},
		})
		err := c.validateMustacheReferences(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_sources.projects.proxy_request.headers.X-Tenant")
	})
}

func TestValidateMustacheReferences_NestedPath(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("nested cfg path resolves on root field", func(t *testing.T) {
		// {{cfg.tenant.name}} requires "tenant" to be declared as a property; the nested
		// "name" lookup is at runtime against whatever shape the property holds.
		c := newOAuthConnector(&AuthOAuth2{
			Type: AuthTypeOAuth2,
			Authorization: AuthOauth2Authorization{
				Endpoint: "https://{{cfg.tenant.name}}.example.com/oauth/authorize",
			},
		}, preconnectFlow(`{"type":"object","properties":{"tenant":{"type":"object"}}}`))
		require.NoError(t, c.validateMustacheReferences(vc))
	})
}

func TestValidateMustacheReferences_NonOAuthSkipped(t *testing.T) {
	vc := &common.ValidationContext{}

	t.Run("api key connector with no data sources is skipped", func(t *testing.T) {
		c := &Connector{
			Labels:      map[string]string{"type": "api"},
			DisplayName: "API",
			Description: "API",
			Auth:        &Auth{InnerVal: &AuthApiKey{Type: AuthTypeAPIKey}},
		}
		require.NoError(t, c.validateMustacheReferences(vc))
	})
}

func TestConnectorValidate_MustacheRefsBubbleUp(t *testing.T) {
	// Belt-and-suspenders: ensure the mustache validation actually runs from the
	// public Validate entrypoint that route handlers call.
	c := newOAuthConnector(&AuthOAuth2{
		Type: AuthTypeOAuth2,
		Authorization: AuthOauth2Authorization{
			Endpoint: "https://{{cfg.tenant}}.example.com/oauth/authorize",
		},
	}, nil)
	err := c.Validate(&common.ValidationContext{})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "{{cfg.tenant}}"), "expected error to mention missing reference, got: %s", err.Error())
}
