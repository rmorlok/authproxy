package oauth2

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
)

func TestSplitScopes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{}},
		{name: "whitespace only", in: "   \t\n", want: []string{}},
		{name: "single", in: "read", want: []string{"read"}},
		{name: "multiple", in: "read write", want: []string{"read", "write"}},
		{name: "extra whitespace", in: "  read   write\twrite2 ", want: []string{"read", "write", "write2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitScopes(tt.in)
			assert.NotNil(t, got, "should never return nil")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJoinScopes(t *testing.T) {
	tests := []struct {
		name string
		in   []config.Scope
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "empty", in: []config.Scope{}, want: ""},
		{name: "single", in: []config.Scope{{Id: "read"}}, want: "read"},
		{name: "multiple", in: []config.Scope{{Id: "read"}, {Id: "write"}}, want: "read write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, JoinScopes(tt.in))
		})
	}
}

func TestScopeSet(t *testing.T) {
	got := scopeSet("read write read")
	assert.Len(t, got, 2, "duplicate tokens should collapse")
	assert.Contains(t, got, "read")
	assert.Contains(t, got, "write")

	assert.Empty(t, scopeSet(""), "empty input yields empty set")
}
