package registry

import (
	"fmt"
	"testing"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestCanonicalResourcesAndLists(t *testing.T) {
	checkRegistration[actor.Actor](t, actor.ActorKind)
	checkRegistration[connection.Connection](t, connection.ConnectionKind)
	checkRegistration[connectors.Connector](t, connectors.ConnectorKind)
	checkRegistration[key.Key](t, key.KeyKind)
	checkRegistration[namespace.Namespace](t, namespace.NamespaceKind)
	checkRegistration[rate_limit.RateLimit](t, rate_limit.RateLimitKind)
	require.Len(t, NewResourceScheme().RegisteredGVKs(), 12)
}

func checkRegistration[T any](t *testing.T, kind meta.Kind) {
	t.Helper()
	t.Run(string(kind), func(t *testing.T) {
		scheme := NewResourceScheme()
		payload := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"name":"example","namespace":"root"},"spec":{}}`, kind)
		resource, err := scheme.DecodeJSON([]byte(payload))
		require.NoError(t, err)
		require.IsType(t, new(T), resource)
		again, err := scheme.DecodeJSON([]byte(payload))
		require.NoError(t, err)
		require.NotSame(t, resource, again)

		listPayload := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{},"items":[%s]}`, apiv1alpha1.ListKind(kind), payload)
		list, err := scheme.DecodeJSON([]byte(listPayload))
		require.NoError(t, err)
		require.IsType(t, new(apiv1alpha1.ResourceList[T]), list)
		require.Len(t, list.(*apiv1alpha1.ResourceList[T]).Items, 1)

		// JSON is also valid YAML; exercise both scheme decoding paths.
		yamlList, err := scheme.DecodeYAML([]byte(listPayload))
		require.NoError(t, err)
		require.Equal(t, list, yamlList)
		badItem := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{},"spec":{},"unknown":true}`, kind)
		_, err = scheme.DecodeJSON([]byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"items":[%s]}`, apiv1alpha1.ListKind(kind), badItem)))
		require.ErrorContains(t, err, "unknown")
	})
}

func TestIndependentSchemesAndUnsupportedContracts(t *testing.T) {
	first, second := NewResourceScheme(), NewResourceScheme()
	gvk := manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "LocalExtension"}
	require.NoError(t, manifest.RegisterType[actor.Actor](first, gvk))
	require.NotContains(t, second.RegisteredGVKs(), gvk)
	for _, payload := range []string{
		`{"apiVersion":"authproxy.net/v999","kind":"Actor"}`,
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"ActorPatch"}`,
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"ConnectorForceState"}`,
	} {
		_, err := second.DecodeJSON([]byte(payload))
		require.Error(t, err)
	}
}
