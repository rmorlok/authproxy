package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPermission_Equal(t *testing.T) {
	tests := []struct {
		name   string
		p1, p2 Permission
		equal  bool
	}{
		{
			name:  "empty",
			p1:    Permission{},
			p2:    Permission{},
			equal: true,
		},
		{
			name: "namespace, resources and verbs",
			p1: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			p2: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			equal: true,
		},
		{
			name: "namespace, resources, resource ids, and verbs",
			p1: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			p2: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			equal: true,
		},
		{
			name: "missing namespace",
			p1: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			p2: Permission{
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "missing resources",
			p1: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			p2: Permission{
				Namespace:   "root",
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "missing resource ids",
			p1: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			p2: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "missing verbs",
			p1: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			p2: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
			},
			equal: false,
		},
		{
			name: "different namespace",
			p1: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			p2: Permission{
				Namespace: "root.foo",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "different resources",
			p1: Permission{
				Namespace: "root",
				Resources: []string{"actors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			p2: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "different verbs",
			p1: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "create"},
			},
			p2: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			equal: false,
		},
		{
			name: "different resource ids",
			p1: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			p2: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"3", "4"},
				Verbs:       []string{"get", "list"},
			},
			equal: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.p1.Equal(tt.p1), "expected p1 equal to itself")
			require.True(t, tt.p2.Equal(tt.p2), "expected p2 equal to itself")
			require.Equal(t, tt.equal, tt.p1.Equal(tt.p2))
		})
	}
}

func TestPermission_Validate(t *testing.T) {
	tests := []struct {
		name  string
		p     Permission
		valid bool
	}{
		{
			name:  "empty",
			p:     Permission{},
			valid: false,
		},
		{
			name: "namespace, resources and verbs",
			p: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			valid: true,
		},
		{
			name: "namespace, resources, resource ids, and verbs",
			p: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: true,
		},
		{
			name: "missing namespace",
			p: Permission{
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "missing resources",
			p: Permission{
				Namespace:   "root",
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "missing resource ids",
			p: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{"get", "list"},
			},
			valid: true, // Resource ids are not required
		},
		{
			name: "missing verbs",
			p: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
			},
			valid: false,
		},
		{
			name: "empty namespace",
			p: Permission{
				Namespace:   "",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "empty resources",
			p: Permission{
				Namespace:   "root",
				Resources:   []string{},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "empty resource ids",
			p: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{},
				Verbs:       []string{"get", "list"},
			},
			valid: true, // Resource ids are not required
		},
		{
			name: "empty verbs",
			p: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{},
			},
			valid: false,
		},
		{
			name: "blank resource",
			p: Permission{
				Namespace:   "root",
				Resources:   []string{""},
				ResourceIds: []string{"1", "2"},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "blank resource ids",
			p: Permission{
				Namespace:   "root",
				Resources:   []string{"connectors", "connections"},
				ResourceIds: []string{""},
				Verbs:       []string{"get", "list"},
			},
			valid: false,
		},
		{
			name: "blank verbs",
			p: Permission{
				Namespace: "root",
				Resources: []string{"connectors", "connections"},
				Verbs:     []string{""},
			},
			valid: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
