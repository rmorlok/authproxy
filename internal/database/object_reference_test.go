package database

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
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

type objectReferenceFixture struct {
	db         DB
	namespace  *Namespace
	actor      *Actor
	connector  *ConnectorWithDefinition
	connection *Connection
	key        *Key
	rateLimit  *RateLimit
}

func newObjectReferenceFixture(t *testing.T) objectReferenceFixture {
	t.Helper()
	ctx := context.Background()
	_, db := MustApplyBlankTestDbConfig(t, nil)

	ns := &Namespace{Path: "root.team"}
	require.NoError(t, db.CreateNamespace(ctx, ns))

	actor := &Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  ns.Path,
		Name:       "operator",
		ExternalId: "operator@example.com",
	}
	require.NoError(t, db.CreateActor(ctx, actor))

	connector := testConnectorWithDefinition(
		apid.New(apid.PrefixConnector),
		ns.Path,
		"billing",
		1,
	)
	require.NoError(t, db.UpsertConnectorDefinitionVersion(ctx, connector))

	connection := &Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        ns.Path,
		Name:             "production",
		State:            ConnectionStateConfigured,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}
	require.NoError(t, db.CreateConnection(ctx, connection))

	key := &Key{
		Id:        apid.New(apid.PrefixKey),
		Namespace: ns.Path,
		Name:      "primary",
	}
	require.NoError(t, db.CreateKey(ctx, key))

	rateLimit := &RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  ns.Path,
		Name:       "api-budget",
		Definition: validDef(),
	}
	require.NoError(t, db.CreateRateLimit(ctx, rateLimit))

	return objectReferenceFixture{
		db:         db,
		namespace:  ns,
		actor:      actor,
		connector:  connector,
		connection: connection,
		key:        key,
		rateLimit:  rateLimit,
	}
}

func objectReference(
	kind meta.Kind,
	id, namespace string,
	name common.ResourceName,
) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       kind,
		ID:         id,
		Namespace:  namespace,
		Name:       name,
	}
}

func TestResolveObjectReferencesBySupportedIdentity(t *testing.T) {
	fixture := newObjectReferenceFixture(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		kind       meta.Kind
		id         string
		namespace  string
		objectName common.ResourceName
		resolve    func(meta.ObjectReference) (string, error)
	}{
		{
			name:       "actor",
			kind:       actorschema.ActorKind,
			id:         fixture.actor.Id.String(),
			namespace:  fixture.actor.Namespace,
			objectName: fixture.actor.Name,
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveActorReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Id.String(), nil
			},
		},
		{
			name:       "connection",
			kind:       connectionschema.ConnectionKind,
			id:         fixture.connection.Id.String(),
			namespace:  fixture.connection.Namespace,
			objectName: fixture.connection.Name,
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveConnectionReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Id.String(), nil
			},
		},
		{
			name:       "connector",
			kind:       connectorschema.ConnectorKind,
			id:         fixture.connector.Id.String(),
			namespace:  fixture.connector.Namespace,
			objectName: fixture.connector.Name,
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveConnectorReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Id.String(), nil
			},
		},
		{
			name:       "key",
			kind:       keyschema.KeyKind,
			id:         fixture.key.Id.String(),
			namespace:  fixture.key.Namespace,
			objectName: fixture.key.Name,
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveKeyReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Id.String(), nil
			},
		},
		{
			name:       "namespace",
			kind:       namespaceschema.NamespaceKind,
			id:         fixture.namespace.Path,
			namespace:  "root",
			objectName: "team",
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveNamespaceReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Path, nil
			},
		},
		{
			name:       "rate limit",
			kind:       ratelimitschema.RateLimitKind,
			id:         fixture.rateLimit.Id.String(),
			namespace:  fixture.rateLimit.Namespace,
			objectName: fixture.rateLimit.Name,
			resolve: func(ref meta.ObjectReference) (string, error) {
				value, err := fixture.db.ResolveRateLimitReference(ctx, ref)
				if err != nil {
					return "", err
				}
				return value.Id.String(), nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			byID, err := test.resolve(objectReference(test.kind, test.id, "", ""))
			require.NoError(t, err)
			require.Equal(t, test.id, byID)

			byName, err := test.resolve(objectReference(
				test.kind,
				"",
				test.namespace,
				test.objectName,
			))
			require.NoError(t, err)
			require.Equal(t, test.id, byName)

			byBoth, err := test.resolve(objectReference(
				test.kind,
				test.id,
				test.namespace,
				test.objectName,
			))
			require.NoError(t, err)
			require.Equal(t, test.id, byBoth)
		})
	}
}

func TestResolveObjectReferenceValidation(t *testing.T) {
	fixture := newObjectReferenceFixture(t)
	ctx := context.Background()

	t.Run("unsupported kind", func(t *testing.T) {
		_, err := fixture.db.(*service).resolveObjectReference(
			ctx,
			objectReference(
				"Widget",  // kind
				"wid_123", // id
				"",        // namespace
				"",        // name
			),
		)
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "unsupported kind")
	})

	t.Run("missing identity", func(t *testing.T) {
		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			keyschema.KeyKind,
			"",
			"",
			"",
		))
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "must contain id or namespace and name")
	})

	t.Run("typed resolver requires its kind", func(t *testing.T) {
		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			connectorschema.ConnectorKind,
			fixture.connector.Id.String(),
			"",
			"",
		))
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "expected kind \"Key\"")
	})

	t.Run("api version", func(t *testing.T) {
		ref := objectReference(keyschema.KeyKind, fixture.key.Id.String(), "", "")
		ref.APIVersion = "authproxy.net/v2"
		_, err := fixture.db.ResolveKeyReference(ctx, ref)
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "apiVersion")
	})

	t.Run("id prefix", func(t *testing.T) {
		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			keyschema.KeyKind,
			fixture.actor.Id.String(),
			"",
			"",
		))
		require.ErrorIs(t, err, ErrInvalidReference)
	})

	t.Run("partial namespaced name", func(t *testing.T) {
		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			keyschema.KeyKind,
			fixture.key.Id.String(),
			fixture.key.Namespace,
			"",
		))
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "supplied together")
	})

	t.Run("namespace matchers are not namespaced identities", func(t *testing.T) {
		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			keyschema.KeyKind,
			"",
			"root.**",
			fixture.key.Name,
		))
		require.ErrorIs(t, err, ErrInvalidReference)
	})

	t.Run("generation only applies to connectors", func(t *testing.T) {
		keyRef := objectReference(keyschema.KeyKind, fixture.key.Id.String(), "", "")
		keyRef.Generation = 1
		_, err := fixture.db.ResolveKeyReference(ctx, keyRef)
		require.ErrorIs(t, err, ErrInvalidReference)

		connectorRef := objectReference(
			connectorschema.ConnectorKind,
			fixture.connector.Id.String(),
			"",
			"",
		)
		connectorRef.Generation = 99
		resolved, err := fixture.db.ResolveConnectorReference(ctx, connectorRef)
		require.NoError(t, err)
		require.Equal(t, fixture.connector.Id, resolved.Id)
	})

	t.Run("id and name must identify the same row", func(t *testing.T) {
		other := &Key{
			Id:        apid.New(apid.PrefixKey),
			Namespace: fixture.key.Namespace,
			Name:      "secondary",
		}
		require.NoError(t, fixture.db.CreateKey(ctx, other))

		_, err := fixture.db.ResolveKeyReference(ctx, objectReference(
			keyschema.KeyKind,
			fixture.key.Id.String(),
			other.Namespace,
			other.Name,
		))
		require.ErrorIs(t, err, ErrInvalidReference)
		require.ErrorContains(t, err, "does not match")
	})
}

func TestResolveObjectReferenceExcludesDeletedRows(t *testing.T) {
	fixture := newObjectReferenceFixture(t)
	ctx := context.Background()
	reference := objectReference(
		ratelimitschema.RateLimitKind,
		fixture.rateLimit.Id.String(),
		"",
		"",
	)

	require.NoError(t, fixture.db.DeleteRateLimit(ctx, fixture.rateLimit.Id))
	_, err := fixture.db.ResolveRateLimitReference(ctx, reference)
	require.ErrorIs(t, err, ErrNotFound)
}
