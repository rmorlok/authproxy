package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
					Id:          apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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
					Id:         apid.New(apid.PrefixActor),
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
				Id:         apid.New(apid.PrefixActor),
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

func TestRequestAuth_GetNamespacesAllowedForResource(t *testing.T) {
	tests := []struct {
		name       string
		ra         *RequestAuth
		resource   string
		verb       string
		namespaces []string
	}{
		{
			name:       "nil RequestAuth",
			ra:         nil,
			resource:   "connections",
			verb:       "get",
			namespaces: nil,
		},
		{
			name:       "unauthenticated",
			ra:         NewUnauthenticatedRequestAuth(),
			resource:   "connections",
			verb:       "get",
			namespaces: nil,
		},
		{
			name: "no permission verb",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.other",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			resource:   "connections",
			verb:       "force_state",
			namespaces: []string{},
		},
		{
			name: "no permission resource",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.other",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			resource:   "actors",
			verb:       "get",
			namespaces: []string{},
		},
		{
			name: "single permission",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.other",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.other"},
		},
		{
			name: "multiple permissions",
			ra: NewAuthenticatedRequestAuth(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			}),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one", "root.two"},
		},
		{
			name: "request permissions no intersection - namespace",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.other", // No intersection
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{},
		},
		{
			name: "request permissions no intersection - verb",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"list"}, // No intersection
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{},
		},
		{
			name: "request permissions no intersection - resource",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.two",
						Resources: []string{"actors"}, // No intersection
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{},
		},
		{
			name: "request permissions single intersection",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.two"},
		},
		{
			name: "request permissions multiple intersection",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one", "root.two"},
		},
		{
			name: "wildcard resource actor permissions",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"*"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one"},
		},
		{
			name: "wildcard resource request permissions",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"*"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one"},
		},
		{
			name: "wildcard verb actor permissions",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"*"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one"},
		},
		{
			name: "wildcard verb request permissions",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.three",
						Resources: []string{"actors"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.one",
						Resources: []string{"connections"},
						Verbs:     []string{"*"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.one"},
		},
		{
			name: "request permissions wildcard",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.child.one",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.child.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.other.three",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.child.**",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.child.one", "root.child.two"},
		},
		{
			name: "actor and request permissions wildcard",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.child.one.**",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.child.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.other.three",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.child.**",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.child.one.**", "root.child.two"},
		},
		{
			name: "actor wildcard resolved to concrete",
			ra: NewAuthenticatedRequestAuthWithPermissions(&Actor{
				Id:         apid.New(apid.PrefixActor),
				ExternalId: "user",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.child.one.**",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.child.two",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.other.three",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.four",
						Resources: []string{"connections"},
						Verbs:     []string{"list"},
					},
				},
			},
				[]aschema.Permission{
					{
						Namespace: "root.child.one.bobcat",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			),
			resource:   "connections",
			verb:       "get",
			namespaces: []string{"root.child.one.bobcat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespaces := tt.ra.GetNamespacesAllowed(tt.resource, tt.verb)
			require.Equal(t, tt.namespaces, namespaces)
		})
	}
}
