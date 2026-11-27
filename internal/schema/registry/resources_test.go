package registry

import (
	"fmt"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// TestCanonicalResourcesAndLists checks fresh typed resource and list factories
// through the canonical JSON and YAML decoding paths.
func TestCanonicalResourcesAndLists(t *testing.T) {
	checkRegistration[actor.Actor](t, actor.ActorKind)
	checkRegistration[connection.Connection](t, connection.ConnectionKind)
	checkRegistration[connectors.Connector](t, connectors.ConnectorKind)
	checkRegistration[key.Key](t, key.KeyKind)
	checkRegistration[namespace.Namespace](t, namespace.NamespaceKind)
	checkRegistration[rate_limit.RateLimit](t, rate_limit.RateLimitKind)
	checkRegistration[tools.Tool](t, tools.ToolKind)
	require.Len(t, NewResourceScheme().RegisteredGVKs(), 14)
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
	require.NoError(t, manifest.RegisterType[actor.Actor](first.Scheme, gvk))
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

// TestResourceCapabilities keeps metadata, ID validation, and strict patch
// decoding attached to each registered canonical resource type.
func TestResourceCapabilities(t *testing.T) {
	cases := []struct {
		kind       meta.Kind
		collection string
		id         string
		patch      any
	}{
		{actor.ActorKind, "actors", apid.New(apid.PrefixActor).String(), new(actor.ActorPatch)},
		{connection.ConnectionKind, "connections", apid.New(apid.PrefixConnection).String(), new(connection.ConnectionPatch)},
		{connectors.ConnectorKind, "connectors", apid.New(apid.PrefixConnector).String(), new(connectors.ConnectorPatch)},
		{key.KeyKind, "keys", apid.New(apid.PrefixKey).String(), new(key.KeyPatch)},
		{namespace.NamespaceKind, "namespaces", "root.example", new(namespace.NamespacePatch)},
		{rate_limit.RateLimitKind, "rate-limits", apid.New(apid.PrefixRateLimit).String(), new(rate_limit.RateLimitPatch)},
		{tools.ToolKind, "tools", apid.New(apid.PrefixTool).String(), new(tools.ToolPatch)},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			scheme := NewResourceScheme()
			descriptor, err := scheme.Lookup(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: tc.kind})
			require.NoError(t, err)
			require.Equal(t, tc.collection, descriptor.Collection)
			require.NoError(t, descriptor.ValidateID(tc.id))
			require.Error(t, descriptor.ValidateID("invalid"))
			require.IsType(t, tc.patch, descriptor.NewPatch())
			require.NotSame(t, descriptor.NewPatch(), descriptor.NewPatch())
			data := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"id":%q,"name":"example","namespace":"root","labels":{"team":"one"}},"spec":{}}`, tc.kind, tc.id)
			resource, err := scheme.DecodeJSON([]byte(data))
			require.NoError(t, err)
			require.IsType(t, resource, descriptor.NewResource())
			inferred, err := TypeOf(resource)
			require.NoError(t, err)
			require.Equal(t, descriptor.GVK, inferred.GVK)
			metadata, err := descriptor.Metadata(resource)
			require.NoError(t, err)
			require.Equal(t, tc.id, metadata.ID)
			metadata.Labels["team"] = "changed"
			unchanged, err := descriptor.Metadata(resource)
			require.NoError(t, err)
			require.Equal(t, "one", unchanged.Labels["team"])
			patchData := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{},"spec":{}}`, tc.kind)
			patch, err := descriptor.DecodePatchJSON([]byte(patchData))
			require.NoError(t, err)
			require.IsType(t, tc.patch, patch)
			_, err = descriptor.DecodePatchJSON([]byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"spec":{}}`, tc.kind)))
			require.Error(t, err, "missing patch metadata must not be defaulted")
			_, err = descriptor.DecodePatchJSON([]byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{},"spec":{},"unknown":true}`, tc.kind)))
			require.Error(t, err)
		})
	}
}

// TestToolResourceCapabilities preserves standalone Tool patch semantics and
// revision status without assigning a published-generation lifecycle.
func TestToolResourceCapabilities(t *testing.T) {
	scheme := NewResourceScheme()
	descriptor, err := scheme.Lookup(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: tools.ToolKind})
	require.NoError(t, err)
	require.Equal(t, tools.SchemaIDToolResource, descriptor.SchemaRef)
	require.Nil(t, descriptor.Generations)

	// A normal authored YAML Tool resolves to the same canonical contract as JSON.
	value, err := scheme.DecodeYAML([]byte(`apiVersion: authproxy.net/v1alpha1
kind: Tool
metadata:
  name: lookup-record
  namespace: root
spec:
  connectionRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    name: provider
    namespace: root
  description: Look up a record
  verbs: ["tool:records.read"]
  inputSchema:
    type: object
    properties:
      id: {type: integer}
  outputSchema: {type: object}
  proxyHttp:
    method: GET
    url: https://example.com/records
`))
	require.NoError(t, err)
	require.NoError(t, descriptor.ValidateResource(value, meta.ValidationModeCreate))
	current := value.(*tools.Tool)
	current.Metadata.ID = apid.New(apid.PrefixTool).String()
	current.Status = &tools.ToolStatus{Revision: 7}

	// Registry patch application delegates explicit clears and whole-plan
	// replacement to ToolPatch, retaining server-owned revision status.
	patch, err := descriptor.DecodePatchJSON([]byte(`{
		"apiVersion":"authproxy.net/v1alpha1","kind":"Tool","metadata":{},
		"spec":{"proxyHttp":null,"javascript":"async function execute() { return {}; }","outputSchema":null}
	}`))
	require.NoError(t, err)
	updated, err := descriptor.ApplyPatch(current, patch)
	require.NoError(t, err)
	candidate := updated.(*tools.Tool)
	require.Nil(t, candidate.Spec.ProxyHTTP)
	require.NotNil(t, candidate.Spec.Javascript)
	require.Nil(t, candidate.Spec.OutputSchema)
	require.Equal(t, uint64(7), candidate.Status.Revision)
	require.Zero(t, candidate.Metadata.Generation)
	require.NotNil(t, current.Spec.ProxyHTTP)
	require.Nil(t, current.Spec.Javascript)
	require.NotEmpty(t, current.Spec.OutputSchema)

	// Status and generation remain read-only/unsupported even through the registry.
	_, err = descriptor.DecodePatchJSON([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Tool","metadata":{},"spec":{},"status":{"revision":8}}`))
	require.ErrorContains(t, err, "status")
	_, err = descriptor.DecodePatchJSON([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Tool","metadata":{"generation":1},"spec":{}}`))
	require.ErrorContains(t, err, "metadata.generation")

	// A generated Tool cannot be edited by applying an ordinary standalone patch.
	current.Status.ManagedBy = &tools.ToolManagedBy{
		ToolSetRef: meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1, Kind: "ToolSet",
			ID: apid.New(apid.PrefixToolSet).String(), Namespace: "root", Generation: 1,
		},
		SourceKey: "lookup-record",
	}
	_, err = descriptor.ApplyPatch(current, patch)
	require.ErrorContains(t, err, "managed by ToolSet")
}

func TestPatchApplicationAndTypeSafety(t *testing.T) {
	scheme := NewResourceScheme()
	d, err := scheme.Lookup(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: actor.ActorKind})
	require.NoError(t, err)
	current, err := scheme.DecodeJSON([]byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"id":%q,"name":"example","namespace":"root"},"spec":{"externalId":"subject"}}`, apid.New(apid.PrefixActor))))
	require.NoError(t, err)
	patch, err := d.DecodePatchJSON([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"labels":{"team":"two"}},"spec":{}}`))
	require.NoError(t, err)
	updated, err := d.ApplyPatch(current, patch)
	require.NoError(t, err)
	require.Equal(t, "two", updated.(*actor.Actor).Metadata.Labels["team"])
	require.Empty(t, current.(*actor.Actor).Metadata.Labels)
	immutable, err := d.DecodePatchJSON([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{},"spec":{"externalId":"other"}}`))
	require.NoError(t, err)
	_, err = d.ApplyPatch(current, immutable)
	require.ErrorContains(t, err, "immutable")
	for _, bad := range []any{nil, (*actor.Actor)(nil), new(key.Key)} {
		_, err = d.Metadata(bad)
		require.Error(t, err)
		_, err = d.ApplyPatch(bad, patch)
		require.Error(t, err)
		require.Error(t, d.ValidateResource(bad, meta.ValidationModeCreate))
	}
	for _, bad := range []any{nil, (*actor.ActorPatch)(nil), new(key.KeyPatch)} {
		_, err = d.ApplyPatch(current, bad)
		require.Error(t, err)
	}
	_, err = TypeOf((*actor.Actor)(nil))
	require.Error(t, err)
	// Lifecycle validation stays with the resource implementation.
	fresh := actor.NewActor()
	fresh.Metadata.Namespace = "root"
	fresh.Spec.ExternalId = "subject"
	require.NoError(t, d.ValidateResource(fresh, meta.ValidationModeCreate))
	fresh.Spec.ExternalId = ""
	require.Error(t, d.ValidateResource(fresh, meta.ValidationModeCreate))
	_, err = LookupResource(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "ActorList"})
	require.Error(t, err)
}
