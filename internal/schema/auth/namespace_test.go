package auth

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
					path:      "root.child",
					expectErr: false,
				},
				{
					name:      "ValidNestedChildPath",
					path:      "root.child.grandchild",
					expectErr: false,
				},
				{
					name:      "EmptyPath",
					path:      "",
					expectErr: true,
				},
				{
					name:      "PathNotStartingWithRoot",
					path:      "notroot.child",
					expectErr: true,
				},
				{
					name:      "PathWithInvalidCharacter",
					path:      "root.child@123",
					expectErr: true,
				},
				{
					name:      "PathOnlyAsterisk",
					path:      "root.*",
					expectErr: true,
				},
				{
					name:      "PathOnlyDoubleAsterisk",
					path:      "root.**",
					expectErr: true,
				},
				{
					name:      "PathStartingWithAsterisk",
					path:      "root.*child",
					expectErr: true,
				},
				{
					name:      "PathStartingWithDoubleAsterisk",
					path:      "root.**child",
					expectErr: true,
				},
				{
					name:      "PathWithAsterisk",
					path:      "root.child*namespace",
					expectErr: true,
				},
				{
					name:      "PathWithDoubleAsterisk",
					path:      "root.child**namespace",
					expectErr: true,
				},
				{
					name:      "PathWithUppercaseLetter",
					path:      "root.Child",
					expectErr: false,
				},
				{
					name:      "PathContainingSpace",
					path:      "root.child with space",
					expectErr: true,
				},
				{
					name:      "PathWithTrailingDot",
					path:      "root.child.",
					expectErr: true,
				},
				{
					name:      "PathWithDoubleDot",
					path:      "root.child..grandchild",
					expectErr: true,
				},
				{
					name:      "PathWithSpecialCharacters",
					path:      "root.child!@#",
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
		t.Run("matcher", func(t *testing.T) {
			tests := []struct {
				name      string
				matcher   string
				expectErr bool
			}{
				{
					name:      "ValidRootPath",
					matcher:   "root",
					expectErr: false,
				},
				{
					name:      "ValidChildPath",
					matcher:   "root.child",
					expectErr: false,
				},
				{
					name:      "ValidNestedChildPath",
					matcher:   "root.child.grandchild",
					expectErr: false,
				},
				{
					name:      "EmptyPath",
					matcher:   "",
					expectErr: true,
				},
				{
					name:      "PathNotStartingWithRoot",
					matcher:   "notroot.child",
					expectErr: true,
				},
				{
					name:      "PathWithInvalidCharacter",
					matcher:   "root.child@123",
					expectErr: true,
				},
				{
					name:      "PathAsterisk",
					matcher:   "root.*",
					expectErr: true,
				},
				{
					name:      "PathDoubleAsterisk",
					matcher:   "root.**",
					expectErr: false,
				},
				{
					name:      "PathStartingWithAsterisk",
					matcher:   "root.*child",
					expectErr: true,
				},
				{
					name:      "PathStartingWithDoubleAsterisk",
					matcher:   "root.**child",
					expectErr: true,
				},
				{
					name:      "PathWithAsterisk",
					matcher:   "root.child*namespace",
					expectErr: true,
				},
				{
					name:      "PathWithDoubleAsterisk",
					matcher:   "root.child**namespace",
					expectErr: true,
				},
				{
					name:      "PathWithUppercaseLetter",
					matcher:   "root.Child",
					expectErr: false,
				},
				{
					name:      "PathContainingSpace",
					matcher:   "root.child with space",
					expectErr: true,
				},
				{
					name:      "PathWithTrailingDot",
					matcher:   "root.child.",
					expectErr: true,
				},
				{
					name:      "PathWithDoubleDot",
					matcher:   "root.child..grandchild",
					expectErr: true,
				},
				{
					name:      "PathWithSpecialCharacters",
					matcher:   "root.child!@#",
					expectErr: true,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					err := ValidateNamespaceMatcher(tt.matcher)
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
					path:     "root.child",
					prefixes: []string{"root", "root.child"},
				},
				{
					name:     "grandchild",
					path:     "root.child.grandchild",
					prefixes: []string{"root", "root.child", "root.child.grandchild"},
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
		t.Run("split multiple", func(t *testing.T) {
			tests := []struct {
				name     string
				paths    []string
				prefixes []string
			}{
				{
					name:     "single root",
					paths:    []string{"root"},
					prefixes: []string{"root"},
				},
				{
					name:     "single child",
					paths:    []string{"root.child"},
					prefixes: []string{"root", "root.child"},
				},
				{
					name:     "single grandchild",
					paths:    []string{"root.child.grandchild"},
					prefixes: []string{"root", "root.child", "root.child.grandchild"},
				},
				{
					name:     "empty",
					paths:    []string{""},
					prefixes: []string{},
				},
				{
					name:     "nil",
					paths:    nil,
					prefixes: []string{},
				},
				{
					name:     "duplicate grandchild",
					paths:    []string{"root.child.grandchild", "root.child.grandchild"},
					prefixes: []string{"root", "root.child", "root.child.grandchild"},
				},
				{
					name:     "different parents",
					paths:    []string{"root.child1.grandchild", "root.child2.grandchild"},
					prefixes: []string{"root", "root.child1", "root.child2", "root.child1.grandchild", "root.child2.grandchild"},
				},
				{
					name:     "multiple levels",
					paths:    []string{"root.child1.grandchild", "root.child1", "root.child3", "root.child2.grandchild"},
					prefixes: []string{"root", "root.child1", "root.child2", "root.child3", "root.child1.grandchild", "root.child2.grandchild"},
				},
				{
					name:     "favors depth before alphabetical order",
					paths:    []string{"root.aaaaaa.grandchild.greatgrandchild", "root.b", "root.b.grandchild", "root.aaaaaa.grandchild"},
					prefixes: []string{"root", "root.aaaaaa", "root.b", "root.aaaaaa.grandchild", "root.b.grandchild", "root.aaaaaa.grandchild.greatgrandchild"},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					prefixes := SplitNamespacePathsToPrefixes(tt.paths)
					if !reflect.DeepEqual(prefixes, tt.prefixes) {
						t.Errorf("expected prefixes %v, got %v", tt.prefixes, prefixes)
					}
				})
			}
		})
		t.Run("NamespacePathFromRoot", func(t *testing.T) {
			require.Equal(t, NamespacePathFromRoot(), RootNamespace)
			require.Equal(t, NamespacePathFromRoot("some-namespace"), RootNamespace+".some-namespace")
			require.Equal(t, NamespacePathFromRoot("some-namespace", "other-namespace"), RootNamespace+".some-namespace.other-namespace")
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
					parent: "root.child",
					child:  "root.child",
					result: false,
				},
				{
					name:   "Child of root",
					parent: "root",
					child:  "root.child",
					result: true,
				},
				{
					name:   "Nested",
					parent: "root.child",
					child:  "root.child.grandchild",
					result: true,
				},
				{
					name:   "Requires separator",
					parent: "root.child",
					child:  "root.childgrandchild",
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
					parent: "root.child",
					child:  "root.child",
					result: true,
				},
				{
					name:   "Child of root",
					parent: "root",
					child:  "root.child",
					result: true,
				},
				{
					name:   "Nested",
					parent: "root.child",
					child:  "root.child.grandchild",
					result: true,
				},
				{
					name:   "Requires separator",
					parent: "root.child",
					child:  "root.childgrandchild",
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
	t.Run("DepthOfNamespacePath", func(t *testing.T) {
		tests := []struct {
			name  string
			path  string
			depth uint64
		}{
			{
				name:  "root",
				path:  "root",
				depth: 0,
			},
			{
				name:  "root with slash",
				path:  "root.",
				depth: 0,
			},
			{
				name:  "single child",
				path:  "root.child",
				depth: 1,
			},
			{
				name:  "single child with slash",
				path:  "root.child.",
				depth: 1,
			},
			{
				name:  "grandchild",
				path:  "root.child.grandchild",
				depth: 2,
			},
			{
				name:  "empty path",
				path:  "",
				depth: 0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Equal(t, tt.depth, DepthOfNamespacePath(tt.path))
			})
		}
	})
}
