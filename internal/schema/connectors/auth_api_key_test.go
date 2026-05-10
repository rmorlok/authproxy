package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAuthApiKey_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		auth *AuthApiKey
		yaml string
	}{
		{
			name: "bearer",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type: ApiKeyPlacementBearer,
				},
			},
			yaml: `
				type: api-key
				placement:
					type: bearer
`,
		},
		{
			name: "header with prefix",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "Authorization",
					Prefix:     "Token ",
				},
			},
			yaml: `
				type: api-key
				placement:
					type: header
					header_name: Authorization
					prefix: 'Token '
`,
		},
		{
			name: "header without prefix",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "X-API-Key",
				},
			},
			yaml: `
				type: api-key
				placement:
					type: header
					header_name: X-API-Key
`,
		},
		{
			name: "query",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:      ApiKeyPlacementQuery,
					ParamName: "appid",
				},
			},
			yaml: `
				type: api-key
				placement:
					type: query
					param_name: appid
`,
		},
		{
			name: "basic",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:          ApiKeyPlacementBasic,
					UsernameField: "account_id",
				},
			},
			yaml: `
				type: api-key
				placement:
					type: basic
					username_field: account_id
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("yaml", func(t *testing.T) {
				wrapper := &Auth{InnerVal: tt.auth}
				data, err := yaml.Marshal(wrapper)
				require.NoError(t, err)
				assert.Equal(
					t,
					util.TabsToSpaces(util.Deindent(tt.yaml), 4),
					util.TabsToSpaces(util.Deindent(string(data)), 4),
				)
				back := &Auth{}
				require.NoError(t, yaml.Unmarshal(data, back))
				require.Equal(t, tt.auth, back.Inner())
			})
			t.Run("json", func(t *testing.T) {
				wrapper := &Auth{InnerVal: tt.auth}
				data, err := json.Marshal(wrapper)
				require.NoError(t, err)
				back := &Auth{}
				require.NoError(t, json.Unmarshal(data, back))
				require.Equal(t, tt.auth, back.Inner())
			})
		})
	}
}

func TestAuthApiKey_Clone(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var a *AuthApiKey
		require.Nil(t, a.Clone())
	})

	t.Run("deep copy of placement", func(t *testing.T) {
		orig := &AuthApiKey{
			Type: AuthTypeAPIKey,
			Placement: &ApiKeyPlacement{
				Type:       ApiKeyPlacementHeader,
				HeaderName: "X-API-Key",
				Prefix:     "Token ",
			},
		}

		clone := orig.Clone().(*AuthApiKey)
		require.Equal(t, orig, clone)
		require.NotSame(t, orig.Placement, clone.Placement)

		clone.Placement.HeaderName = "X-Other"
		require.Equal(t, "X-API-Key", orig.Placement.HeaderName)
	})

	t.Run("nil placement", func(t *testing.T) {
		orig := &AuthApiKey{Type: AuthTypeAPIKey}
		clone := orig.Clone().(*AuthApiKey)
		require.Equal(t, orig, clone)
		require.Nil(t, clone.Placement)
	})
}

func TestAuthApiKey_Validate(t *testing.T) {
	tests := []struct {
		name        string
		auth        *AuthApiKey
		wantErrSubs []string
	}{
		{
			name: "nil receiver",
			auth: nil,
		},
		{
			name: "valid bearer",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
			},
		},
		{
			name: "valid header",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "X-API-Key",
				},
			},
		},
		{
			name: "valid header with prefix",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "Authorization",
					Prefix:     "Token ",
				},
			},
		},
		{
			name: "valid query",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:      ApiKeyPlacementQuery,
					ParamName: "api_key",
				},
			},
		},
		{
			name: "valid basic",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:          ApiKeyPlacementBasic,
					UsernameField: "account_id",
				},
			},
		},
		{
			name: "missing placement",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
			},
			wantErrSubs: []string{"placement", "is required"},
		},
		{
			name: "missing placement type",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{},
			},
			wantErrSubs: []string{"placement.type", "is required"},
		},
		{
			name: "unknown placement type",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{Type: ApiKeyPlacementType("aws-sigv4")},
			},
			wantErrSubs: []string{"placement.type", "aws-sigv4", "is not a valid api-key placement type"},
		},
		{
			name: "header missing header_name",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{Type: ApiKeyPlacementHeader},
			},
			wantErrSubs: []string{"placement.header_name", "is required"},
		},
		{
			name: "header with invalid header_name (space)",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "X API Key",
				},
			},
			wantErrSubs: []string{"placement.header_name", "is not a valid HTTP header name"},
		},
		{
			name: "header with invalid header_name (newline)",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "X-API\nKey",
				},
			},
			wantErrSubs: []string{"placement.header_name", "is not a valid HTTP header name"},
		},
		{
			name: "query missing param_name",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{Type: ApiKeyPlacementQuery},
			},
			wantErrSubs: []string{"placement.param_name", "is required"},
		},
		{
			name: "query with invalid param_name (space)",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:      ApiKeyPlacementQuery,
					ParamName: "api key",
				},
			},
			wantErrSubs: []string{"placement.param_name", "is not a valid URL query parameter name"},
		},
		{
			name: "basic missing username_field",
			auth: &AuthApiKey{
				Type:      AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBasic},
			},
			wantErrSubs: []string{"placement.username_field", "is required"},
		},
		{
			name: "bearer with stray header_name",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementBearer,
					HeaderName: "X-API-Key",
				},
			},
			wantErrSubs: []string{"placement.header_name", "is only valid when placement.type is \"header\""},
		},
		{
			name: "bearer with stray prefix",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:   ApiKeyPlacementBearer,
					Prefix: "Token ",
				},
			},
			wantErrSubs: []string{"placement.prefix", "is only valid when placement.type is \"header\""},
		},
		{
			name: "header with stray param_name",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:       ApiKeyPlacementHeader,
					HeaderName: "X-API-Key",
					ParamName:  "api_key",
				},
			},
			wantErrSubs: []string{"placement.param_name", "is only valid when placement.type is \"query\""},
		},
		{
			name: "query with stray username_field",
			auth: &AuthApiKey{
				Type: AuthTypeAPIKey,
				Placement: &ApiKeyPlacement{
					Type:          ApiKeyPlacementQuery,
					ParamName:     "api_key",
					UsernameField: "account_id",
				},
			},
			wantErrSubs: []string{"placement.username_field", "is only valid when placement.type is \"basic\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate(&common.ValidationContext{})
			if len(tt.wantErrSubs) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			msg := err.Error()
			for _, sub := range tt.wantErrSubs {
				assert.Contains(t, msg, sub)
			}
		})
	}
}

func TestAuthApiKey_ValidatorInterface(t *testing.T) {
	// Compile-time check kept here as a runtime smoke too.
	var _ AuthValidator = (*AuthApiKey)(nil)
	a := &AuthApiKey{
		Type:      AuthTypeAPIKey,
		Placement: &ApiKeyPlacement{Type: ApiKeyPlacementBearer},
	}
	require.NoError(t, AuthValidator(a).Validate(&common.ValidationContext{}))
}
