package core

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/rmorlok/authproxy/internal/auth_methods/api_key"
	"github.com/rmorlok/authproxy/internal/auth_methods/no_auth"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// TestGetAuthMethodFactory_ReturnsRegisteredFactoryForEachAuthType verifies
// the registry resolves each auth type to a factory of the right concrete
// type. NewCoreService is the canonical builder; reaching for the per-method
// extras (oauth2-specific NewOAuth2, etc.) through the typed accessors must
// stay in lock-step with what the registry holds.
func TestGetAuthMethodFactory_ReturnsRegisteredFactoryForEachAuthType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _, _, _, _, _ := FullMockService(t, ctrl)
	svc.authMethodFactories = svc.buildAuthMethodFactories()

	cases := []struct {
		name     string
		auth     cschema.AuthImpl
		assertOk func(t *testing.T, got interface{})
	}{
		{
			name: "oauth2",
			auth: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2},
			assertOk: func(t *testing.T, got interface{}) {
				_, ok := got.(oauth2.Factory)
				assert.True(t, ok, "expected oauth2.Factory, got %T", got)
			},
		},
		{
			name: "api_key",
			auth: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey},
			assertOk: func(t *testing.T, got interface{}) {
				_, ok := got.(api_key.Factory)
				assert.True(t, ok, "expected api_key.Factory, got %T", got)
			},
		},
		{
			name: "no_auth",
			auth: &cschema.AuthNoAuth{Type: cschema.AuthTypeNoAuth},
			assertOk: func(t *testing.T, got interface{}) {
				_, ok := got.(no_auth.Factory)
				assert.True(t, ok, "expected no_auth.Factory, got %T", got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := &cschema.Connector{Auth: &cschema.Auth{InnerVal: tc.auth}}
			factory := svc.getAuthMethodFactory(connector)
			assert.NotNil(t, factory)
			tc.assertOk(t, factory)
		})
	}
}

func TestGetAuthMethodFactory_NilOrMissingReturnsNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _, _, _, _, _ := FullMockService(t, ctrl)

	assert.Nil(t, svc.getAuthMethodFactory(nil))
	assert.Nil(t, svc.getAuthMethodFactory(&cschema.Connector{}))
	// Unknown type with an empty registry resolves to nil.
	assert.Nil(t, svc.getAuthMethodFactory(&cschema.Connector{
		Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
	}))
}
