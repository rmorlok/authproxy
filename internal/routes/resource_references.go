package routes

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

var searchResourceTypeByKind = map[meta.Kind]database.SearchResourceType{
	actorschema.ActorKind:           database.SearchResourceTypeActor,
	connectionschema.ConnectionKind: database.SearchResourceTypeConnection,
	connectorschema.ConnectorKind:   database.SearchResourceTypeConnector,
	namespaceschema.NamespaceKind:   database.SearchResourceTypeNamespace,
	keyschema.KeyKind:               database.SearchResourceTypeKey,
	ratelimitschema.RateLimitKind:   database.SearchResourceTypeRateLimit,
}

var searchResourceKindByType = func() map[database.SearchResourceType]meta.Kind {
	result := make(map[database.SearchResourceType]meta.Kind, len(searchResourceTypeByKind))
	for kind, resourceType := range searchResourceTypeByKind {
		result[resourceType] = kind
	}
	return result
}()

func resourceKindForStoredType(resourceType string) (meta.Kind, error) {
	kind, ok := searchResourceKindByType[database.SearchResourceType(resourceType)]
	if !ok {
		return "", fmt.Errorf("unsupported resource type %q", resourceType)
	}
	return kind, nil
}

func searchResourceReference(resource database.SearchResource) (meta.ObjectReference, error) {
	kind, err := resourceKindForStoredType(string(resource.ResourceType))
	if err != nil {
		return meta.ObjectReference{}, err
	}

	metadata := meta.ObjectMeta{
		ID:        resource.ResourceID,
		Name:      common.ResourceName(resource.Name),
		Namespace: resource.Namespace,
	}
	if kind == namespaceschema.NamespaceKind {
		if err := namespaceschema.ValidatePath(resource.ResourceID); err != nil {
			return meta.ObjectReference{}, fmt.Errorf("build namespace reference: %w", err)
		}
		// Namespace names permit values that the shared ResourceName primitive
		// does not. Its immutable path is already the complete identity, so keep
		// the heterogeneous reference ID-only instead of emitting an invalid or
		// duplicated display identity.
		metadata = meta.ObjectMeta{ID: resource.ResourceID}
	}

	return meta.NewObjectReference(meta.NewTypeMeta(kind), metadata), nil
}
