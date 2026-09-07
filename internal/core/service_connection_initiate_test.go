package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInferConnectionNamespace(t *testing.T) {
	tests := []struct {
		name               string
		connectorNamespace string
		actorNamespace     string
		allowedNamespaces  []string
		wantNamespace      string
		wantInferred       bool
	}{
		{
			name:               "uses one exact child namespace",
			connectorNamespace: "root.demo",
			actorNamespace:     "root.demo",
			allowedNamespaces:  []string{"root.demo.demo-user"},
			wantNamespace:      "root.demo.demo-user",
			wantInferred:       true,
		},
		{
			name:               "rejects namespace matcher",
			connectorNamespace: "root.demo",
			actorNamespace:     "root.demo",
			allowedNamespaces:  []string{"root.demo.**"},
		},
		{
			name:               "rejects ambiguous namespaces",
			connectorNamespace: "root.demo",
			actorNamespace:     "root.demo",
			allowedNamespaces:  []string{"root.demo.alice", "root.demo.bob"},
		},
		{
			name:               "rejects namespace outside connector",
			connectorNamespace: "root.demo",
			actorNamespace:     "root",
			allowedNamespaces:  []string{"root.other.alice"},
		},
		{
			name:               "rejects namespace outside actor",
			connectorNamespace: "root",
			actorNamespace:     "root.demo",
			allowedNamespaces:  []string{"root.other.alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotInferred := inferConnectionNamespace(
				tt.connectorNamespace,
				tt.actorNamespace,
				tt.allowedNamespaces,
			)
			require.Equal(t, tt.wantNamespace, gotNamespace)
			require.Equal(t, tt.wantInferred, gotInferred)
		})
	}
}
