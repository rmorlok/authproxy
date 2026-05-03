package oauth2

import (
	"testing"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
)

func TestDetectScopeMismatch(t *testing.T) {
	required := func(id string) sconfig.Scope { return sconfig.Scope{Id: id} }
	optional := func(id string) sconfig.Scope {
		f := false
		return sconfig.Scope{Id: id, Required: &f}
	}

	tests := []struct {
		name             string
		declared         []sconfig.Scope
		granted          string
		wantRequired     []string
		wantOptional     []string
		wantExtraGranted []string
	}{
		{
			name:     "exact match",
			declared: []sconfig.Scope{required("read"), required("write")},
			granted:  "read write",
		},
		{
			name:         "required missing",
			declared:     []sconfig.Scope{required("read"), required("write")},
			granted:      "read",
			wantRequired: []string{"write"},
		},
		{
			name:         "optional missing",
			declared:     []sconfig.Scope{required("read"), optional("write")},
			granted:      "read",
			wantOptional: []string{"write"},
		},
		{
			name:     "optional present, required satisfied",
			declared: []sconfig.Scope{required("read"), optional("write")},
			granted:  "read write",
		},
		{
			name:             "extra granted",
			declared:         []sconfig.Scope{required("read")},
			granted:          "read admin",
			wantExtraGranted: []string{"admin"},
		},
		{
			name:             "mixed",
			declared:         []sconfig.Scope{required("read"), required("write"), optional("delete")},
			granted:          "read admin",
			wantRequired:     []string{"write"},
			wantOptional:     []string{"delete"},
			wantExtraGranted: []string{"admin"},
		},
		{
			name:     "empty granted falls back to nothing",
			declared: []sconfig.Scope{required("read")},
			granted:  "",
			// scopes string is empty; token_response.go sets scopes=requestedScopes
			// when provider omits, so this shape only happens if provider explicitly
			// returned empty. Treat as "nothing granted".
			wantRequired: []string{"read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectScopeMismatch(tt.declared, tt.granted)
			assert.Equal(t, tt.wantRequired, got.missingRequired, "missingRequired")
			assert.Equal(t, tt.wantOptional, got.missingOptional, "missingOptional")
			assert.Equal(t, tt.wantExtraGranted, got.extraGranted, "extraGranted")
		})
	}
}
