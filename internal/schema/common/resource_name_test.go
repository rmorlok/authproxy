package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceNameValidate(t *testing.T) {
	tests := []struct {
		name  string
		value ResourceName
		valid bool
	}{
		{name: "single character", value: "a", valid: true},
		{name: "generated AuthProxy id", value: "act_01jz6y5ke2nq9gc0fj8x7r3d4m", valid: true},
		{name: "label compatible punctuation", value: "Prod.us_east-1", valid: true},
		{name: "maximum length", value: ResourceName(strings.Repeat("a", ResourceNameMaxLength)), valid: true},
		{name: "empty", value: "", valid: false},
		{name: "too long", value: ResourceName(strings.Repeat("a", ResourceNameMaxLength+1)), valid: false},
		{name: "starts with punctuation", value: "-invalid", valid: false},
		{name: "ends with punctuation", value: "invalid_", valid: false},
		{name: "contains whitespace", value: "not valid", valid: false},
		{name: "contains slash", value: "not/valid", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestResourceNamePreservesCase(t *testing.T) {
	name := ResourceName("Production")
	require.NoError(t, name.Validate())
	require.Equal(t, "Production", string(name))
	require.NotEqual(t, ResourceName("production"), name)
}
