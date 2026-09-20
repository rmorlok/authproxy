package registry

import (
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenerationCapabilityRegistration(t *testing.T) {
	for _, kind := range []meta.Kind{"Actor", "Connection", "Connector", "Key", "Namespace", "RateLimit"} {
		descriptor, err := LookupResource(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: kind})
		require.NoError(t, err)
		if kind == "Connector" {
			require.NotNil(t, descriptor.Generations)
		} else {
			require.Nil(t, descriptor.Generations)
		}
	}
}

func TestGenerationCapabilityRejectsWrongTypes(t *testing.T) {
	descriptor, err := LookupResource(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "Connector"})
	require.NoError(t, err)
	g := descriptor.Generations
	current := connectors.NewConnector()
	patch := connectors.NewConnectorPatch()
	for _, bad := range []any{nil, (*connectors.Connector)(nil), &actor.Actor{}} {
		_, err = g.State(bad)
		require.Error(t, err)
		_, err = g.Select(bad, current, false, false)
		require.Error(t, err)
		_, err = g.Select(current, bad, false, false)
		require.Error(t, err)
		_, err = g.Finalize(bad, current, patch, nil, false)
		require.Error(t, err)
		_, err = g.Finalize(current, bad, patch, nil, false)
		require.Error(t, err)
	}
	for _, bad := range []any{nil, (*connectors.ConnectorPatch)(nil), current} {
		_, err = g.ChangesGeneration(bad)
		require.Error(t, err)
		_, err = g.Finalize(current, current, bad, nil, false)
		require.Error(t, err)
	}
}
