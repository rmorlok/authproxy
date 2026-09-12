package apply

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/util"
)

type lifecycleResource interface {
	ValidateFor(meta.ValidationMode, *common.ValidationContext) error
}

// Adapters contain operation contracts, not resource registrations. Resource
// decoding continues to use the shared schema registry.
type resourceAdapter struct {
	collection string
	patch      func() lifecycleResource
	merge      func(any, lifecycleResource) error
}

func adapterFor(kind meta.Kind) (resourceAdapter, error) {
	switch kind {
	case actor.ActorKind:
		return resourceAdapter{"actors", func() lifecycleResource { return actor.NewActorPatch() }, func(current any, p lifecycleResource) error {
			_, err := p.(*actor.ActorPatch).ApplyTo(current.(*actor.Actor), nil)
			return err
		}}, nil
	case namespace.NamespaceKind:
		return resourceAdapter{"namespaces", func() lifecycleResource { return namespace.NewNamespacePatch() }, func(current any, p lifecycleResource) error {
			_, err := p.(*namespace.NamespacePatch).ApplyTo(current.(*namespace.Namespace), nil)
			return err
		}}, nil
	case key.KeyKind:
		return resourceAdapter{"keys", func() lifecycleResource { return key.NewKeyPatch() }, func(current any, p lifecycleResource) error {
			_, err := p.(*key.KeyPatch).ApplyTo(current.(*key.Key), nil)
			return err
		}}, nil
	case connectors.ConnectorKind:
		return resourceAdapter{"connectors", func() lifecycleResource { return new(connectors.ConnectorPatch) }, func(current any, p lifecycleResource) error {
			_, err := p.(*connectors.ConnectorPatch).ApplyTo(current.(*connectors.Connector), nil)
			return err
		}}, nil
	case rate_limit.RateLimitKind:
		return resourceAdapter{"rate-limits", func() lifecycleResource { return rate_limit.NewRateLimitPatch() }, func(current any, p lifecycleResource) error {
			_, err := p.(*rate_limit.RateLimitPatch).ApplyTo(current.(*rate_limit.RateLimit), nil)
			return err
		}}, nil
	case connection.ConnectionKind:
		return resourceAdapter{"connections", func() lifecycleResource { return new(connection.ConnectionPatch) }, func(current any, p lifecycleResource) error {
			_, err := current.(*connection.Connection).ApplyUpdate(p.(*connection.ConnectionPatch))
			return err
		}}, nil
	}
	return resourceAdapter{}, fmt.Errorf("unsupported apply kind %q", kind)
}

func resourceMetadata(resource any) (meta.ObjectMeta, meta.Kind, error) {
	switch r := resource.(type) {
	case *actor.Actor:
		return r.Metadata, r.Kind, nil
	case *namespace.Namespace:
		return r.Metadata, r.Kind, nil
	case *key.Key:
		return r.Metadata, r.Kind, nil
	case *connectors.Connector:
		return r.Metadata, r.Kind, nil
	case *rate_limit.RateLimit:
		return r.Metadata, r.Kind, nil
	case *connection.Connection:
		return r.Metadata, r.Kind, nil
	}
	return meta.ObjectMeta{}, "", fmt.Errorf("expected a canonical resource")
}

// decodePatch preserves presence using the canonical patch decoder. Callers
// supply a calculated patch, not the entire live resource (which may be masked).
func decodePatch(kind meta.Kind, data []byte, current any) (lifecycleResource, error) {
	adapter, err := adapterFor(kind)
	if err != nil {
		return nil, err
	}
	patch := adapter.patch()
	if err := util.DecodeJSONStrict(data, patch); err != nil {
		return nil, fmt.Errorf("invalid %s patch fields or types", kind)
	}
	if err := apserde.ValidateNoRedactedPlaceholders(patch); err != nil {
		return nil, fmt.Errorf("redacted placeholders cannot be submitted")
	}
	if err := patch.ValidateFor(meta.ValidationModeUpdate, nil); err != nil {
		return nil, fmt.Errorf("%s patch failed semantic validation", kind)
	}
	_, currentKind, err := resourceMetadata(current)
	if err != nil || currentKind != kind {
		return nil, fmt.Errorf("patch target kind mismatch")
	}
	if err := adapter.merge(current, patch); err != nil {
		return nil, fmt.Errorf("%s patch changes immutable identity/policy or produces an invalid resource", kind)
	}
	return patch, nil
}
