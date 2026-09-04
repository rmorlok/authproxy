package core

import (
	"context"
	"errors"
	"fmt"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

func coreObjectReferenceError(kind meta.Kind, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, database.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, database.ErrInvalidReference) {
		return fmt.Errorf("%w: invalid %s reference: %v", ErrInvalidArgument, kind, err)
	}
	return err
}

// ResolveActorReference resolves an actor and converts the persistence model
// to the actor object used by authentication and authorization code.
func (s *service) ResolveActorReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*authcore.Actor, error) {
	actor, err := s.db.ResolveActorReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	return authcore.CreateActor(actor), nil
}

// ResolveConnectionReference resolves a connection and hydrates its pinned
// connector definition version before returning the core object.
func (s *service) ResolveConnectionReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (iface.Connection, error) {
	connection, err := s.db.ResolveConnectionReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	return s.getConnectionForDb(ctx, connection)
}

// ResolveConnectorReference resolves the logical connector first, then uses
// generation to choose the requested definition version. An omitted
// generation selects the newest version.
func (s *service) ResolveConnectorReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (iface.Connector, error) {
	connector, err := s.db.ResolveConnectorReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}

	if reference.Generation != 0 {
		return s.getConnectorVersion(ctx, connector.Id, reference.Generation)
	}

	version, err := s.db.NewestConnectorDefinitionVersionForId(ctx, connector.Id)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	result := wrapConnector(*version, s)
	if _, err := result.getDefinition(); err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveKeyReference resolves and wraps a managed key.
func (s *service) ResolveKeyReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (iface.Key, error) {
	key, err := s.db.ResolveKeyReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	return wrapKey(*key, s), nil
}

// ResolveNamespaceReference resolves and wraps a namespace. Namespace IDs are
// canonical paths; namespace/name identities use the parent path and segment.
func (s *service) ResolveNamespaceReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (iface.Namespace, error) {
	namespace, err := s.db.ResolveNamespaceReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	return wrapNamespace(*namespace, s), nil
}

// ResolveRateLimitReference resolves and wraps a rate limit.
func (s *service) ResolveRateLimitReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (iface.RateLimit, error) {
	rateLimit, err := s.db.ResolveRateLimitReference(ctx, reference)
	if err != nil {
		return nil, coreObjectReferenceError(reference.Kind, err)
	}
	return wrapRateLimit(*rateLimit, s), nil
}
