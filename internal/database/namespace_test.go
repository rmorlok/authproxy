package database

import (
	"testing"
)

func TestValidateNamespacePath(t *testing.T) {
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
}
