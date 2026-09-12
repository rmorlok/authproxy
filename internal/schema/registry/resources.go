// Package registry assembles AuthProxy's canonical resource types for callers
// that dispatch manifests by apiVersion and kind.
package registry

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// NewResourceScheme returns an independent scheme containing every canonical
// resource and its typed list envelope. Registration recognizes wire types;
// callers must separately enforce operation support and lifecycle validation.
// Patches and actions are deliberately excluded: patches share a resource's
// GVK but require an operation-specific decoder.
func NewResourceScheme() *manifest.Scheme {
	scheme := manifest.NewScheme()
	registerResource[actor.Actor](scheme, actor.ActorKind)
	registerResource[connection.Connection](scheme, connection.ConnectionKind)
	registerResource[connectors.Connector](scheme, connectors.ConnectorKind)
	registerResource[key.Key](scheme, key.KeyKind)
	registerResource[namespace.Namespace](scheme, namespace.NamespaceKind)
	registerResource[rate_limit.RateLimit](scheme, rate_limit.RateLimitKind)
	return scheme
}

func registerResource[T any](scheme *manifest.Scheme, kind meta.Kind) {
	manifest.MustRegisterType[T](scheme, manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: kind})
	manifest.MustRegisterType[apiv1alpha1.ResourceList[T]](scheme, manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: apiv1alpha1.ListKind(kind)})
}
