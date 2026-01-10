package core

import (
	"testing"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestPermission_Allows(t *testing.T) {
	tests := []struct {
		name       string
		p          aschema.Permission
		namespace  string
		resource   string
		verb       string
		resourceId string
		allowed    bool
	}{
		// Exact namespace matching
		{
			name: "exact namespace match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "exact namespace no match",
			p: aschema.Permission{
				Namespace: "root.foo",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "exact namespace child no match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root.foo",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},

		// Wildcard namespace matching
		{
			name: "wildcard namespace matches base",
			p: aschema.Permission{
				Namespace: "root.**",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "wildcard namespace matches child",
			p: aschema.Permission{
				Namespace: "root.**",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root.foo",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "wildcard namespace matches deep child",
			p: aschema.Permission{
				Namespace: "root.**",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root.foo.bar.baz",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "wildcard namespace partial match",
			p: aschema.Permission{
				Namespace: "root.foo.**",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root.foo.bar",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "wildcard namespace no match sibling",
			p: aschema.Permission{
				Namespace: "root.foo.**",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root.bar",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},

		// Resource matching
		{
			name: "resource match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections", "connectors"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "resource no match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connectors"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "resource wildcard",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"*"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},

		// Verb matching
		{
			name: "verb match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get", "list"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "verb no match",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"list"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "verb wildcard",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"*"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "delete",
			allowed:   true,
		},

		// Resource ID matching
		{
			name: "no resource ids allows any",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "abc-123",
			allowed:    true,
		},
		{
			name: "resource id match",
			p: aschema.Permission{
				Namespace:   "root",
				Resources:   []string{"connections"},
				ResourceIds: []string{"abc-123", "def-456"},
				Verbs:       []string{"get"},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "abc-123",
			allowed:    true,
		},
		{
			name: "resource id no match",
			p: aschema.Permission{
				Namespace:   "root",
				Resources:   []string{"connections"},
				ResourceIds: []string{"abc-123"},
				Verbs:       []string{"get"},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "xyz-789",
			allowed:    false,
		},
		{
			name: "resource ids specified but empty request id allowed (list operation)",
			p: aschema.Permission{
				Namespace:   "root",
				Resources:   []string{"connections"},
				ResourceIds: []string{"abc-123"},
				Verbs:       []string{"list"},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "list",
			resourceId: "",
			allowed:    true,
		},

		// Full wildcard permission
		{
			name: "full wildcard admin permission",
			p: aschema.Permission{
				Namespace: "root.**",
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			namespace:  "root.foo.bar",
			resource:   "anything",
			verb:       "everything",
			resourceId: "any-id",
			allowed:    true,
		},

		// Edge cases
		{
			name: "empty namespace in request",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "empty resource in request",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "empty verb in request",
			p: aschema.Permission{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"get"},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "",
			allowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allows(tt.p, tt.namespace, tt.resource, tt.verb, tt.resourceId)
			require.Equal(t, tt.allowed, result)
		})
	}
}

func TestPermissionsAllow(t *testing.T) {
	tests := []struct {
		name        string
		permissions []aschema.Permission
		namespace   string
		resource    string
		verb        string
		resourceId  string
		allowed     bool
	}{
		{
			name:        "empty permissions",
			permissions: []aschema.Permission{},
			namespace:   "root",
			resource:    "connections",
			verb:        "get",
			allowed:     false,
		},
		{
			name:        "nil permissions",
			permissions: nil,
			namespace:   "root",
			resource:    "connections",
			verb:        "get",
			allowed:     false,
		},
		{
			name: "single matching permission",
			permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "multiple permissions first matches",
			permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
				{
					Namespace: "root",
					Resources: []string{"connectors"},
					Verbs:     []string{"list"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "multiple permissions second matches",
			permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connectors"},
					Verbs:     []string{"list"},
				},
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "multiple permissions none match",
			permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connectors"},
					Verbs:     []string{"list"},
				},
				{
					Namespace: "root.other",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "additive permissions combine resources",
			permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
				{
					Namespace: "root",
					Resources: []string{"connectors"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connectors",
			verb:      "get",
			allowed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PermissionsAllow(tt.permissions, tt.namespace, tt.resource, tt.verb, tt.resourceId)
			require.Equal(t, tt.allowed, result)
		})
	}
}

func TestPermissionsAllowWithRestrictions(t *testing.T) {
	tests := []struct {
		name             string
		actorPermissions []aschema.Permission
		restrictions     []aschema.Permission
		namespace        string
		resource         string
		verb             string
		resourceId       string
		allowed          bool
	}{
		{
			name: "actor allowed, no restrictions",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			restrictions: nil,
			namespace:    "root",
			resource:     "connections",
			verb:         "get",
			allowed:      true,
		},
		{
			name: "actor allowed, empty restrictions",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			restrictions: []aschema.Permission{},
			namespace:    "root",
			resource:     "connections",
			verb:         "get",
			allowed:      true,
		},
		{
			name:             "actor not allowed",
			actorPermissions: []aschema.Permission{},
			restrictions:     nil,
			namespace:        "root",
			resource:         "connections",
			verb:             "get",
			allowed:          false,
		},
		{
			name: "actor allowed, restrictions allowed",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace: "root.foo",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root.foo",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "actor allowed, restrictions deny different namespace",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace: "root.foo",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root.bar",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "actor allowed, restrictions deny different resource",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connectors",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "actor allowed, restrictions deny different verb",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "delete",
			allowed:   false,
		},
		{
			name: "restrictions are additive within themselves",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace: "root.foo",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
				{
					Namespace: "root.bar",
					Resources: []string{"connectors"},
					Verbs:     []string{"list"},
				},
			},
			namespace: "root.bar",
			resource:  "connectors",
			verb:      "list",
			allowed:   true,
		},
		{
			name: "restriction with resource ids",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace:   "root",
					Resources:   []string{"connections"},
					ResourceIds: []string{"abc-123"},
					Verbs:       []string{"get"},
				},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "abc-123",
			allowed:    true,
		},
		{
			name: "restriction with resource ids denies other ids",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			restrictions: []aschema.Permission{
				{
					Namespace:   "root",
					Resources:   []string{"connections"},
					ResourceIds: []string{"abc-123"},
					Verbs:       []string{"get"},
				},
			},
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "xyz-789",
			allowed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PermissionsAllowWithRestrictions(
				tt.actorPermissions,
				tt.restrictions,
				tt.namespace,
				tt.resource,
				tt.verb,
				tt.resourceId,
			)
			require.Equal(t, tt.allowed, result)
		})
	}
}
