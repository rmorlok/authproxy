package routes

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestSearchResourceReferenceSupportsEverySearchKind(t *testing.T) {
	tests := []struct {
		resourceType database.SearchResourceType
		kind         meta.Kind
		id           string
	}{
		{database.SearchResourceTypeActor, actorschema.ActorKind, "act_test0000000000001"},
		{database.SearchResourceTypeConnection, connectionschema.ConnectionKind, "cxn_test0000000000001"},
		{database.SearchResourceTypeConnector, connectorschema.ConnectorKind, "cxr_test0000000000001"},
		{database.SearchResourceTypeKey, keyschema.KeyKind, "key_test0000000000001"},
		{database.SearchResourceTypeRateLimit, ratelimitschema.RateLimitKind, "rl_test00000000000001"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			ref, err := searchResourceReference(database.SearchResource{
				ResourceType: tt.resourceType,
				ResourceID:   tt.id,
				Name:         "example",
				Namespace:    "root.acme",
			})
			require.NoError(t, err)
			require.Equal(t, meta.APIVersionV1Alpha1, ref.APIVersion)
			require.Equal(t, tt.kind, ref.Kind)
			require.Equal(t, tt.id, ref.ID)
			require.Equal(t, common.ResourceName("example"), ref.Name)
			require.Equal(t, "root.acme", ref.Namespace)
		})
	}
}

func TestSearchResourceReferenceUsesNamespacePathIdentity(t *testing.T) {
	ref, err := searchResourceReference(database.SearchResource{
		ResourceType: database.SearchResourceTypeNamespace,
		ResourceID:   "root.acme.team",
		Name:         "team",
		Namespace:    "root.acme.team",
	})
	require.NoError(t, err)
	require.Equal(t, namespaceschema.NamespaceKind, ref.Kind)
	require.Equal(t, "root.acme.team", ref.ID)
	require.Empty(t, ref.Name)
	require.Empty(t, ref.Namespace)
}

func TestSearchResourceReferenceRejectsUnknownType(t *testing.T) {
	_, err := searchResourceReference(database.SearchResource{ResourceType: "request_event"})
	require.ErrorContains(t, err, "unsupported resource type")
}
