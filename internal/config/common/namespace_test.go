package common

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamespaces(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		t.Run("validation", func(t *testing.T) {
			tests := []struct {
				name      string
				path      string
				expectErr bool
			}{
				{
					name:      "ValidRootPath",
					path:      "root",
					expectErr: false,
				},
				{
					name:      "ValidChildPath",
					path:      "root/child",
					expectErr: false,
				},
				{
					name:      "ValidNestedChildPath",
					path:      "root/child/grandchild",
					expectErr: false,
				},
				{
					name:      "EmptyPath",
					path:      "",
					expectErr: true,
				},
				{
					name:      "PathNotStartingWithRoot",
					path:      "notroot/child",
					expectErr: true,
				},
				{
					name:      "PathWithInvalidCharacter",
					path:      "root/child@123",
					expectErr: true,
				},
				{
					name:      "PathWithUppercaseLetter",
					path:      "root/Child",
					expectErr: false,
				},
				{
					name:      "PathContainingSpace",
					path:      "root/child with space",
					expectErr: true,
				},
				{
					name:      "PathWithTrailingSlash",
					path:      "root/child/",
					expectErr: true,
				},
				{
					name:      "PathWithSpecialCharacters",
					path:      "root/child!@#",
					expectErr: true,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					err := ValidateNamespacePath(tt.path)
					if tt.expectErr {
						if err == nil {
							t.Errorf("expected error but got nil")
						}
					} else {
						if err != nil {
							t.Errorf("did not expect error but got: %v", err)
						}
					}
				})
			}
		})
		t.Run("splitting", func(t *testing.T) {
			tests := []struct {
				name     string
				path     string
				prefixes []string
			}{
				{
					name:     "root",
					path:     "root",
					prefixes: []string{"root"},
				},
				{
					name:     "single child",
					path:     "root/child",
					prefixes: []string{"root", "root/child"},
				},
				{
					name:     "grandchild",
					path:     "root/child/grandchild",
					prefixes: []string{"root", "root/child", "root/child/grandchild"},
				},
				{
					name:     "empty path",
					path:     "",
					prefixes: []string{},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					prefixes := SplitNamespacePathToPrefixes(tt.path)
					if !reflect.DeepEqual(prefixes, tt.prefixes) {
						t.Errorf("expected prefixes %v, got %v", tt.prefixes, prefixes)
					}
				})
			}
		})
		t.Run("NamespacePathFromRoot", func(t *testing.T) {
			require.Equal(t, NamespacePathFromRoot(), RootNamespace)
			require.Equal(t, NamespacePathFromRoot("some-namespace"), RootNamespace+"/some-namespace")
			require.Equal(t, NamespacePathFromRoot("some-namespace", "other-namespace"), RootNamespace+"/some-namespace/other-namespace")
		})
		t.Run("NamespaceIsChild", func(t *testing.T) {
			tests := []struct {
				name   string
				parent string
				child  string
				result bool
			}{
				{
					name:   "Empty Child",
					parent: "root",
					child:  "",
					result: false,
				},
				{
					name:   "Empty Parent",
					parent: "",
					child:  "root",
					result: false,
				},
				{
					name:   "Same root",
					parent: "root",
					child:  "root",
					result: false,
				},
				{
					name:   "Same child",
					parent: "root/child",
					child:  "root/child",
					result: false,
				},
				{
					name:   "Child of root",
					parent: "root",
					child:  "root/child",
					result: true,
				},
				{
					name:   "Nested",
					parent: "root/child",
					child:  "root/child/grandchild",
					result: true,
				},
				{
					name:   "Requires separator",
					parent: "root/child",
					child:  "root/childgrandchild",
					result: false,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := NamespaceIsChild(tt.parent, tt.child)
					require.Equal(t, tt.result, result)
				})
			}
		})
		t.Run("NamespaceIsSameOrChild", func(t *testing.T) {
			tests := []struct {
				name   string
				parent string
				child  string
				result bool
			}{
				{
					name:   "Empty Child",
					parent: "root",
					child:  "",
					result: false,
				},
				{
					name:   "Empty Parent",
					parent: "",
					child:  "root",
					result: false,
				},
				{
					name:   "Same root",
					parent: "root",
					child:  "root",
					result: true,
				},
				{
					name:   "Same child",
					parent: "root/child",
					child:  "root/child",
					result: true,
				},
				{
					name:   "Child of root",
					parent: "root",
					child:  "root/child",
					result: true,
				},
				{
					name:   "Nested",
					parent: "root/child",
					child:  "root/child/grandchild",
					result: true,
				},
				{
					name:   "Requires separator",
					parent: "root/child",
					child:  "root/childgrandchild",
					result: false,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result := NamespaceIsSameOrChild(tt.parent, tt.child)
					require.Equal(t, tt.result, result)
				})
			}
		})
	})
}
