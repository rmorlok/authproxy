package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMustGetValidatorFromContext(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		expectErr string
	}{
		{
			name:      "validator exists in context",
			ctx:       (&ResourcePermissionValidator{}).ContextWith(context.Background()),
			expectErr: "",
		},
		{
			name:      "validator not in context",
			ctx:       context.Background(),
			expectErr: "no resource validator present in context",
		},
		{
			name: "incorrect type in context",
			ctx: context.WithValue(
				context.Background(),
				validatorContextKey,
				"not a validator",
			),
			expectErr: "no resource validator present in context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					err := fmt.Sprintf("%v", r)
					if tt.expectErr != "" {
						assert.Equal(t, tt.expectErr, err)
					} else {
						t.Errorf("unexpected panic: %s", err)
					}
				}
			}()

			validator := MustGetValidatorFromContext(tt.ctx)
			if tt.expectErr == "" {
				assert.NotNil(t, validator)
			}
		})
	}
}

type fakeNamespaceObject struct {
	namespace string
}

func (fno *fakeNamespaceObject) GetNamespace() string {
	return fno.namespace
}
func (fno *fakeNamespaceObject) GetId() uuid.UUID {
	return uuid.New()
}

type fakeIdObject struct {
	fakeNamespaceObject
	id uuid.UUID
}

func (fio *fakeIdObject) GetId() uuid.UUID {
	return fio.id
}

type fakeNamespaceNoId struct {
}

func (fno *fakeNamespaceNoId) GetNamespace() string {
	return "some-namespace"
}

func TestResourcePermissionValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		resource    string
		verb        string
		permissions []aschema.Permission
		idExtractor func(interface{}) string
		inputObj    interface{}
		expectErr   error
	}{
		{
			name:        "valid namespace without resource IDs",
			resource:    "connections",
			verb:        "get",
			permissions: []aschema.Permission{{Namespace: "root.namespace1", Resources: []string{"connections"}, Verbs: []string{"get"}}},
			inputObj: &fakeNamespaceObject{
				namespace: "root.namespace1",
			},
			expectErr: nil,
		},
		{
			name:        "namespace not allowed",
			resource:    "connections",
			verb:        "get",
			permissions: []aschema.Permission{{Namespace: "root.namespace1", Resources: []string{"connections"}, Verbs: []string{"get"}}},
			inputObj: &fakeNamespaceObject{
				namespace: "root.namespace2",
			},
			expectErr: errors.New("permission denied: actor permissions do not allow this action"),
		},
		{
			name:     "valid namespace with valid resource ID",
			resource: "connections",
			verb:     "get",
			permissions: []aschema.Permission{
				{
					Namespace:   "root.namespace1",
					Resources:   []string{"connections"},
					ResourceIds: []string{"123e4567-e89b-12d3-a456-426614174000"},
					Verbs:       []string{"get"},
				},
			},
			inputObj: &fakeIdObject{
				fakeNamespaceObject: fakeNamespaceObject{namespace: "root.namespace1"},
				id:                  uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			},
			expectErr: nil,
		},
		{
			name:     "valid namespace with invalid resource ID",
			resource: "connections",
			verb:     "get",
			permissions: []aschema.Permission{
				{
					Namespace:   "root.namespace1",
					Resources:   []string{"connections"},
					ResourceIds: []string{"123e4567-e89b-12d3-a456-426614174000"},
					Verbs:       []string{"get"},
				},
			},
			inputObj: &fakeIdObject{
				fakeNamespaceObject: fakeNamespaceObject{namespace: "root.namespace1"},
				id:                  uuid.MustParse("123e4567-e89b-12d3-a456-426614174001"),
			},
			expectErr: errors.New("permission denied: actor permissions do not allow this action"),
		},
		{
			name:     "custom id extractor allowed",
			resource: "connections",
			verb:     "get",
			permissions: []aschema.Permission{
				{
					Namespace:   "root.namespace1",
					Resources:   []string{"connections"},
					ResourceIds: []string{"custom-id"},
					Verbs:       []string{"get"},
				},
			},
			idExtractor: func(obj interface{}) string {
				return "custom-id"
			},
			inputObj: &fakeIdObject{
				fakeNamespaceObject: fakeNamespaceObject{namespace: "root.namespace1"},
				id:                  uuid.MustParse("123e4567-e89b-12d3-a456-426614174001"),
			},
			expectErr: nil,
		},
		{
			name:        "namespace retrieval panic",
			resource:    "connections",
			verb:        "get",
			permissions: []aschema.Permission{{Namespace: "root.namespace1", Resources: []string{"connections"}, Verbs: []string{"get"}}},
			inputObj:    struct{}{},
			expectErr:   errors.New("object does not implement namespace retrieval"),
		},
		{
			name:        "id retrieval panic",
			resource:    "connections",
			verb:        "get",
			permissions: []aschema.Permission{{Namespace: "root.namespace1", Resources: []string{"connections"}, Verbs: []string{"get"}}},
			inputObj:    &fakeNamespaceNoId{},
			expectErr:   errors.New("could not extract id from object"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &ResourcePermissionValidator{
				ra: core.NewAuthenticatedRequestAuth(&core.Actor{
					Permissions: tt.permissions,
				}),
				pvb: &PermissionValidatorBuilder{
					resource:    tt.resource,
					verb:        tt.verb,
					idExtractor: tt.idExtractor,
				},
			}

			defer func() {
				if r := recover(); r != nil {
					err := r.(string)
					require.NotNil(t, tt.expectErr)
					assert.Equal(t, tt.expectErr.Error(), err)
				}
			}()

			err := validator.Validate(tt.inputObj)
			require.True(t, validator.hasBeenValidated)

			httpErr := validator.ValidateHttpStatusError(tt.inputObj)

			if tt.expectErr != nil && err != nil {
				assert.Contains(t, err.Error(), tt.expectErr.Error())
			} else {
				assert.Equal(t, tt.expectErr, err)
			}

			if err != nil {
				assert.Error(t, httpErr)
			} else {
				assert.Nil(t, httpErr)
			}
		})
	}
}

type fakeModel struct {
	namespace string
	id        uuid.UUID
}

func (fm *fakeModel) GetNamespace() string {
	return fm.namespace
}
func (fm *fakeModel) GetId() uuid.UUID {
	return fm.id
}

func TestGetEffectiveNamespaceMatchers(t *testing.T) {
	tests := []struct {
		name         string
		authenticated bool
		permissions  []aschema.Permission
		resource     string
		verb         string
		queryMatcher *string
		expected     []string
	}{
		{
			name:          "unauthenticated returns no match sentinel",
			authenticated: false,
			permissions:   nil,
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{aschema.NamespaceNoMatchSentinel},
		},
		{
			name:          "authenticated with no matching permissions returns empty",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connectors"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{},
		},
		{
			name:          "authenticated with matching permission returns namespace",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{"root.prod"},
		},
		{
			name:          "wildcard namespace permission",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{"root.prod.**"},
		},
		{
			name:          "multiple namespace permissions",
			authenticated: true,
			permissions: []aschema.Permission{
				{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}},
				{Namespace: "root.staging", Resources: []string{"connections"}, Verbs: []string{"list"}},
			},
			resource:     "connections",
			verb:         "list",
			queryMatcher: nil,
			expected:     []string{"root.prod", "root.staging"},
		},
		{
			name:          "query matcher intersects with permission",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  strPtr("root.prod.tenant1"),
			expected:      []string{"root.prod.tenant1"},
		},
		{
			name:          "query matcher with wildcard intersects",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  strPtr("root.prod.tenant1.**"),
			expected:      []string{"root.prod.tenant1.**"},
		},
		{
			name:          "query matcher no intersection returns no match sentinel",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  strPtr("root.staging"),
			expected:      []string{aschema.NamespaceNoMatchSentinel},
		},
		{
			name:          "wildcard resource permission",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod", Resources: []string{"*"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{"root.prod"},
		},
		{
			name:          "wildcard verb permission",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"*"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{"root.prod"},
		},
		{
			name:          "permission for different verb returns empty",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"get"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  nil,
			expected:      []string{},
		},
		{
			name:          "multiple permissions with partial match",
			authenticated: true,
			permissions: []aschema.Permission{
				{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}},
				{Namespace: "root.staging", Resources: []string{"connectors"}, Verbs: []string{"list"}}, // Different resource
			},
			resource:     "connections",
			verb:         "list",
			queryMatcher: nil,
			expected:     []string{"root.prod.**"},
		},
		{
			name:          "query matcher constrains wildcard to exact",
			authenticated: true,
			permissions:   []aschema.Permission{{Namespace: "root.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:      "connections",
			verb:          "list",
			queryMatcher:  strPtr("root.prod"),
			expected:      []string{"root.prod"},
		},
		{
			name:          "multiple permissions with query intersection",
			authenticated: true,
			permissions: []aschema.Permission{
				{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}},
				{Namespace: "root.staging.**", Resources: []string{"connections"}, Verbs: []string{"list"}},
			},
			resource:     "connections",
			verb:         "list",
			queryMatcher: strPtr("root.prod.tenant1"),
			expected:     []string{"root.prod.tenant1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ra *core.RequestAuth
			if tt.authenticated {
				ra = core.NewAuthenticatedRequestAuth(&core.Actor{
					Permissions: tt.permissions,
				})
			} else {
				ra = core.NewUnauthenticatedRequestAuth()
			}

			validator := &ResourcePermissionValidator{
				ra: ra,
				pvb: &PermissionValidatorBuilder{
					resource: tt.resource,
					verb:     tt.verb,
				},
			}

			result := validator.GetEffectiveNamespaceMatchers(tt.queryMatcher)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func strPtr(s string) *string {
	return &s
}

// filterTestResource implements hasNamespace and hasId for testing FilterForValidatedResources
type filterTestResource struct {
	namespace string
	id        uuid.UUID
}

func (r *filterTestResource) GetNamespace() string {
	return r.namespace
}

func (r *filterTestResource) GetId() uuid.UUID {
	return r.id
}

func TestFilterForValidatedResources(t *testing.T) {
	tests := []struct {
		name               string
		permissions        []aschema.Permission
		resource           string
		verb               string
		inputNamespaces    []string
		expectedNamespaces []string
	}{
		{
			name:               "filter out resources not in allowed namespace",
			permissions:        []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:           "connections",
			verb:               "list",
			inputNamespaces:    []string{"root.prod", "root.staging", "root.prod"},
			expectedNamespaces: []string{"root.prod", "root.prod"},
		},
		{
			name:               "wildcard namespace matches child namespaces",
			permissions:        []aschema.Permission{{Namespace: "root.prod.**", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:           "connections",
			verb:               "list",
			inputNamespaces:    []string{"root.prod", "root.prod.tenant1", "root.staging"},
			expectedNamespaces: []string{"root.prod", "root.prod.tenant1"},
		},
		{
			name: "multiple namespace permissions",
			permissions: []aschema.Permission{
				{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}},
				{Namespace: "root.staging", Resources: []string{"connections"}, Verbs: []string{"list"}},
			},
			resource:           "connections",
			verb:               "list",
			inputNamespaces:    []string{"root.prod", "root.staging", "root.dev"},
			expectedNamespaces: []string{"root.prod", "root.staging"},
		},
		{
			name:               "empty input returns empty",
			permissions:        []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:           "connections",
			verb:               "list",
			inputNamespaces:    []string{},
			expectedNamespaces: []string{},
		},
		{
			name:               "no matching namespaces returns empty",
			permissions:        []aschema.Permission{{Namespace: "root.prod", Resources: []string{"connections"}, Verbs: []string{"list"}}},
			resource:           "connections",
			verb:               "list",
			inputNamespaces:    []string{"root.staging", "root.dev"},
			expectedNamespaces: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ra := core.NewAuthenticatedRequestAuth(&core.Actor{
				Permissions: tt.permissions,
			})

			validator := &ResourcePermissionValidator{
				ra: ra,
				pvb: &PermissionValidatorBuilder{
					resource: tt.resource,
					verb:     tt.verb,
				},
			}

			input := make([]*filterTestResource, len(tt.inputNamespaces))
			for i, ns := range tt.inputNamespaces {
				input[i] = &filterTestResource{namespace: ns, id: uuid.New()}
			}

			result := FilterForValidatedResources(validator, input)

			assert.Equal(t, len(tt.expectedNamespaces), len(result))
			for i, r := range result {
				assert.Equal(t, tt.expectedNamespaces[i], r.namespace)
			}
			assert.True(t, validator.hasBeenValidated)
		})
	}
}

func TestValidatorOnRoutes(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Auth     A
		Cfg      config.C
		AuthUtil *AuthTestUtil
	}

	setup := func(t *testing.T, register func(g gin.IRouter, auth A)) *TestSetup {
		cfg := config.FromRoot(&sconfig.Root{
			Connectors: &sconfig.Connectors{
				LoadFromList: []sconfig.Connector{},
			},
		})

		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, auth, authUtil := TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)

		r := gin.New() // No recovery middleware to allow panics through
		register(r, auth)

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("validation setup correctly", func(t *testing.T) {
		fakeRouteThatValidates := func(gctx *gin.Context) {
			fm := &fakeModel{
				namespace: "root.test",
				id:        uuid.MustParse("10000000-0000-0000-0000-000000000001"),
			}

			val := MustGetValidatorFromGinContext(gctx)
			httpErr := val.ValidateHttpStatusError(fm)
			if httpErr != nil {
				httpErr.WriteGinResponse(nil, gctx)
				return
			}

			gctx.PureJSON(200, gin.H{"status": "ok"})
		}

		register := func(g gin.IRouter, auth A) {
			g.GET(
				"/cats",
				auth.NewRequiredBuilder().
					ForResource("cats").
					ForVerb("meow").
					Build(),
				fakeRouteThatValidates,
			)
		}

		tu := setup(t, register)

		t.Run("forbidden - namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.prod.**", "cats", "meow"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden - resource id", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "cats", "meow", "10000000-0000-0000-0000-000000000002"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden - verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "cats", "woof"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden - resource", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "dogs", "meow"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "cats", "meow"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp gin.H
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "ok", resp["status"])
		})

		t.Run("valid - resource", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "cats", "meow", "10000000-0000-0000-0000-000000000001"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp gin.H
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "ok", resp["status"])
		})
	})

	t.Run("no in-route validation", func(t *testing.T) {
		fakeRouteThatValidates := func(gctx *gin.Context) {

			// No resource validation

			gctx.PureJSON(200, gin.H{"status": "ok"})
		}

		register := func(g gin.IRouter, auth A) {
			g.GET(
				"/cats",
				auth.NewRequiredBuilder().
					ForResource("cats").
					ForVerb("meow").
					Build(),
				fakeRouteThatValidates,
			)
		}

		tu := setup(t, register)

		t.Run("panics", func(t *testing.T) {
			panicOccurred := false
			defer func() {
				if r := recover(); r != nil {
					panicOccurred = true
				}
			}()

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "cats", "meow"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)

			require.True(t, panicOccurred)
		})

		t.Run("panics with resource id", func(t *testing.T) {
			panicOccurred := false
			defer func() {
				if r := recover(); r != nil {
					panicOccurred = true
				}
			}()

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/cats",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "cats", "meow", "10000000-0000-0000-0000-000000000001"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)

			require.True(t, panicOccurred)
		})
	})
}
