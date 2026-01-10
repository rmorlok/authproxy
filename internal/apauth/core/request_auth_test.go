package core

import (
	"testing"

	"github.com/google/uuid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestRequestAuth_Allows(t *testing.T) {
	tests := []struct {
		name       string
		ra         *RequestAuth
		namespace  string
		resource   string
		verb       string
		resourceId string
		allowed    bool
	}{
		{
			name:      "nil RequestAuth",
			ra:        nil,
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name:      "unauthenticated",
			ra:        NewUnauthenticatedRequestAuth(),
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "actor with matching permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "actor without matching permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connectors"},
						Verbs:     []string{"list"},
					},
				},
			}),
			namespace: "root",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "actor with wildcard namespace permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.**",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			namespace: "root.foo.bar",
			resource:  "connections",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "actor with wildcard resource permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"*"},
						Verbs:     []string{"get"},
					},
				},
			}),
			namespace: "root",
			resource:  "anything",
			verb:      "get",
			allowed:   true,
		},
		{
			name: "actor with wildcard verb permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connections"},
						Verbs:     []string{"*"},
					},
				},
			}),
			namespace: "root",
			resource:  "connections",
			verb:      "delete",
			allowed:   true,
		},
		{
			name: "actor with resource id restriction - allowed",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace:   "root",
						Resources:   []string{"connections"},
						ResourceIds: []string{"abc-123"},
						Verbs:       []string{"get"},
					},
				},
			}),
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "abc-123",
			allowed:    true,
		},
		{
			name: "actor with resource id restriction - denied",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace:   "root",
						Resources:   []string{"connections"},
						ResourceIds: []string{"abc-123"},
						Verbs:       []string{"get"},
					},
				},
			}),
			namespace:  "root",
			resource:   "connections",
			verb:       "get",
			resourceId: "xyz-789",
			allowed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ra.Allows(tt.namespace, tt.resource, tt.verb, tt.resourceId)
			require.Equal(t, tt.allowed, result)
		})
	}
}

func TestRequestAuth_AllowsWithRequestPermissions(t *testing.T) {
	tests := []struct {
		name               string
		actorPermissions   []aschema.Permission
		requestPermissions []aschema.Permission
		namespace          string
		resource           string
		verb               string
		resourceId         string
		allowed            bool
	}{
		{
			name: "actor allowed, no request permissions",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: nil,
			namespace:          "root.foo",
			resource:           "connections",
			verb:               "get",
			allowed:            true,
		},
		{
			name: "actor allowed, request permissions also allowed",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: []aschema.Permission{
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
			name: "actor allowed, request permissions restrict namespace",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: []aschema.Permission{
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
			name: "actor allowed, request permissions restrict resource",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			namespace: "root",
			resource:  "connectors",
			verb:      "get",
			allowed:   false,
		},
		{
			name: "actor allowed, request permissions restrict verb",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"get", "list"},
				},
			},
			namespace: "root",
			resource:  "connections",
			verb:      "delete",
			allowed:   false,
		},
		{
			name: "actor allowed, request permissions restrict resource id",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"*"},
				},
			},
			requestPermissions: []aschema.Permission{
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
		{
			name: "actor not allowed, request permissions would allow",
			actorPermissions: []aschema.Permission{
				{
					Namespace: "root.other",
					Resources: []string{"connections"},
					Verbs:     []string{"get"},
				},
			},
			requestPermissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
			namespace: "root.foo",
			resource:  "connections",
			verb:      "get",
			allowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ra := NewAuthenticatedRequestAuthWithPermissions(
				&Actor{
					Id:          uuid.New(),
					ExternalId:  "user",
					Permissions: tt.actorPermissions,
				},
				tt.requestPermissions,
			)

			result := ra.Allows(tt.namespace, tt.resource, tt.verb, tt.resourceId)
			require.Equal(t, tt.allowed, result)
		})
	}
}

func TestRequestAuth_AllowsReason(t *testing.T) {
	tests := []struct {
		name           string
		ra             *RequestAuth
		namespace      string
		resource       string
		verb           string
		resourceId     string
		allowed        bool
		reasonContains string
	}{
		{
			name:           "nil RequestAuth",
			ra:             nil,
			namespace:      "root",
			resource:       "connections",
			verb:           "get",
			allowed:        false,
			reasonContains: "nil",
		},
		{
			name:           "unauthenticated",
			ra:             NewUnauthenticatedRequestAuth(),
			namespace:      "root",
			resource:       "connections",
			verb:           "get",
			allowed:        false,
			reasonContains: "not authenticated",
		},
		{
			name: "actor permissions deny",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.other",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			namespace:      "root",
			resource:       "connections",
			verb:           "get",
			allowed:        false,
			reasonContains: "actor permissions",
		},
		{
			name: "request permissions deny",
			ra: NewAuthenticatedRequestAuthWithPermissions(
				&Actor{
					Id:         uuid.New(),
					ExternalId: "user",
					Permissions: []aschema.Permission{
						{
							Namespace: "root.**",
							Resources: []string{"*"},
							Verbs:     []string{"*"},
						},
					},
				},
				[]aschema.Permission{
					{
						Namespace: "root.foo",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			namespace:      "root.bar",
			resource:       "connections",
			verb:           "get",
			allowed:        false,
			reasonContains: "request permissions",
		},
		{
			name: "allowed - empty reason",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         uuid.New(),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			namespace:      "root",
			resource:       "connections",
			verb:           "get",
			allowed:        true,
			reasonContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := tt.ra.AllowsReason(tt.namespace, tt.resource, tt.verb, tt.resourceId)
			require.Equal(t, tt.allowed, allowed)
			if tt.reasonContains != "" {
				require.Contains(t, reason, tt.reasonContains)
			} else {
				require.Empty(t, reason)
			}
		})
	}
}
