// Package registry assembles AuthProxy's canonical resource types for callers
// that dispatch manifests by apiVersion and kind.
package registry

import (
	"fmt"
	"reflect"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/util"
)

// Value is the lifecycle validation contract shared by resources and patches.
type Value interface {
	ValidateFor(meta.ValidationMode, *common.ValidationContext) error
}

// ResourceType describes reusable schema capabilities. Operation policy and
// transport execution belong to callers. Descriptors are returned by value.
type ResourceType struct {
	GVK          manifest.GVK
	Collection   string
	NewResource  func() Value
	NewPatch     func() Value
	ValidateID   func(string) error
	resourceType reflect.Type
	metadata     func(any) (meta.ObjectMeta, error)
	applyPatch   func(any, any) (Value, error)
	newList      manifest.Factory
}

// Metadata returns detached metadata after checking the concrete resource type
// and GVK. Wrong types and typed nil pointers produce errors, never panics.
func (r ResourceType) Metadata(resource any) (meta.ObjectMeta, error) {
	return r.metadata(resource)
}

// ValidateResource delegates lifecycle semantics to the canonical schema.
func (r ResourceType) ValidateResource(resource any, mode meta.ValidationMode) error {
	if _, err := r.Metadata(resource); err != nil {
		return err
	}
	return resource.(Value).ValidateFor(mode, nil)
}

// DecodePatchJSON explicitly selects the patch contract for this GVK. Resource
// decoding stays on Scheme.DecodeJSON; the two contracts share a GVK.
func (r ResourceType) DecodePatchJSON(data []byte) (Value, error) {
	patch := r.NewPatch()
	if err := util.DecodeJSONStrict(data, patch); err != nil {
		return nil, err
	}
	if err := patch.ValidateFor(meta.ValidationModeUpdate, nil); err != nil {
		return nil, err
	}
	return patch, nil
}

// ApplyPatch validates and applies a patch using the resource package's own
// implementation. The original resource is not modified.
func (r ResourceType) ApplyPatch(resource, patch any) (Value, error) {
	if _, err := r.Metadata(resource); err != nil {
		return nil, err
	}
	return r.applyPatch(resource, patch)
}

// ResourceScheme composes generic decoding with resource capabilities. It has
// no HTTP client or application policy. Each constructor creates a fresh scheme.
type ResourceScheme struct{ *manifest.Scheme }

func NewResourceScheme() *ResourceScheme {
	scheme := manifest.NewScheme()
	for _, r := range resourceTypes() {
		resource := r
		if err := scheme.Register(r.GVK, func() any { return resource.NewResource() }); err != nil {
			panic(err)
		}
		if err := scheme.Register(manifest.GVK{APIVersion: r.GVK.APIVersion, Kind: apiv1alpha1.ListKind(r.GVK.Kind)}, r.newList); err != nil {
			panic(err)
		}
	}
	return &ResourceScheme{Scheme: scheme}
}

// Lookup returns canonical capabilities; registering a custom decoder on the
// embedded scheme does not give that type resource or mutation capabilities.
func (s *ResourceScheme) Lookup(gvk manifest.GVK) (ResourceType, error) { return LookupResource(gvk) }

// LookupResource provides schema capabilities without allocating a decoder.
func LookupResource(gvk manifest.GVK) (ResourceType, error) {
	for _, r := range resourceTypes() {
		if r.GVK == gvk {
			return r, nil
		}
	}
	return ResourceType{}, fmt.Errorf("unsupported resource %s", gvk)
}

// TypeOf identifies a concrete canonical resource without a caller-side switch.
func TypeOf(resource any) (ResourceType, error) {
	for _, r := range resourceTypes() {
		if reflect.TypeOf(resource) == r.resourceType {
			if _, err := r.Metadata(resource); err != nil {
				return ResourceType{}, err
			}
			return r, nil
		}
	}
	return ResourceType{}, fmt.Errorf("expected a canonical resource")
}

// This is the sole catalogue of resource, list, patch and metadata bindings.
// Constructing descriptor values avoids a mutable global registry.
func resourceTypes() []ResourceType {
	return []ResourceType{
		describe[actor.Actor, actor.ActorPatch](actor.ActorKind, "actors", actor.ValidateID,
			func(r *actor.Actor) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *actor.Actor, patch *actor.ActorPatch) (*actor.Actor, error) {
				return patch.ApplyTo(current, nil)
			}),
		describe[connection.Connection, connection.ConnectionPatch](connection.ConnectionKind, "connections", connection.ValidateID,
			func(r *connection.Connection) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *connection.Connection, patch *connection.ConnectionPatch) (*connection.Connection, error) {
				return current.ApplyUpdate(patch)
			}),
		describe[connectors.Connector, connectors.ConnectorPatch](connectors.ConnectorKind, "connectors", connectors.ValidateID,
			func(r *connectors.Connector) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *connectors.Connector, patch *connectors.ConnectorPatch) (*connectors.Connector, error) {
				return patch.ApplyTo(current, nil)
			}),
		describe[key.Key, key.KeyPatch](key.KeyKind, "keys", key.ValidateID,
			func(r *key.Key) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *key.Key, patch *key.KeyPatch) (*key.Key, error) { return patch.ApplyTo(current, nil) }),
		describe[namespace.Namespace, namespace.NamespacePatch](namespace.NamespaceKind, "namespaces", namespace.ValidatePath,
			func(r *namespace.Namespace) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *namespace.Namespace, patch *namespace.NamespacePatch) (*namespace.Namespace, error) {
				return patch.ApplyTo(current, nil)
			}),
		describe[rate_limit.RateLimit, rate_limit.RateLimitPatch](rate_limit.RateLimitKind, "rate-limits", rate_limit.ValidateID,
			func(r *rate_limit.RateLimit) (meta.TypeMeta, meta.ObjectMeta) { return r.TypeMeta, r.Metadata },
			func(current *rate_limit.RateLimit, patch *rate_limit.RateLimitPatch) (*rate_limit.RateLimit, error) {
				return patch.ApplyTo(current, nil)
			}),
	}
}

func describe[R, P any](kind meta.Kind, collection string, validateID func(string) error,
	metadata func(*R) (meta.TypeMeta, meta.ObjectMeta), merge func(*R, *P) (*R, error)) ResourceType {
	return ResourceType{
		GVK: manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: kind}, Collection: collection,
		NewResource: func() Value { return any(new(R)).(Value) },
		NewPatch:    func() Value { return any(new(P)).(Value) },
		ValidateID:  validateID, resourceType: reflect.TypeOf(new(R)),
		newList: func() any { return new(apiv1alpha1.ResourceList[R]) },
		metadata: func(value any) (meta.ObjectMeta, error) {
			r, ok := value.(*R)
			if !ok || r == nil {
				return meta.ObjectMeta{}, fmt.Errorf("expected a non-nil %s resource", kind)
			}
			tm, m := metadata(r)
			if err := meta.ValidateTypeMeta(tm, meta.APIVersionV1Alpha1, kind, nil); err != nil {
				return meta.ObjectMeta{}, err
			}
			return meta.CloneObjectMeta(m), nil
		},
		applyPatch: func(current, patch any) (Value, error) {
			r, ok := current.(*R)
			if !ok || r == nil {
				return nil, fmt.Errorf("expected a non-nil %s resource", kind)
			}
			p, ok := patch.(*P)
			if !ok || p == nil {
				return nil, fmt.Errorf("expected a non-nil %s patch", kind)
			}
			result, err := merge(r, p)
			if err != nil {
				return nil, err
			}
			return any(result).(Value), nil
		},
	}
}
