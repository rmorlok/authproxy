package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apid"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// objectReferenceRow is implemented by the flat database models that can be
// addressed by a Kubernetes-style object reference. Keeping this interface
// private prevents persistence scanning details from becoming part of DB's
// public contract.
type objectReferenceRow interface {
	cols() []string
	fields() []any
}

type objectReferenceTarget struct {
	table                   string
	idColumn                string
	newRow                  func() objectReferenceRow
	rowID                   func(objectReferenceRow) string
	idValidator             func(string) error
	allowGeneration         bool
	namespacedNameCondition func(meta.ObjectReference) (sq.Sqlizer, error)
}

var objectReferenceTargets = map[meta.Kind]objectReferenceTarget{
	actorschema.ActorKind: {
		table:       ActorTable,
		idColumn:    "id",
		newRow:      func() objectReferenceRow { return &Actor{} },
		rowID:       func(row objectReferenceRow) string { return row.(*Actor).Id.String() },
		idValidator: idValidatorForPrefix(apid.PrefixActor),
	},
	connectionschema.ConnectionKind: {
		table:       ConnectionsTable,
		idColumn:    "id",
		newRow:      func() objectReferenceRow { return &Connection{} },
		rowID:       func(row objectReferenceRow) string { return row.(*Connection).Id.String() },
		idValidator: idValidatorForPrefix(apid.PrefixConnection),
	},
	connectorschema.ConnectorKind: {
		table:           ConnectorsTable,
		idColumn:        "id",
		newRow:          func() objectReferenceRow { return &Connector{} },
		rowID:           func(row objectReferenceRow) string { return row.(*Connector).Id.String() },
		idValidator:     idValidatorForPrefix(apid.PrefixConnector),
		allowGeneration: true,
	},
	keyschema.KeyKind: {
		table:       KeysTable,
		idColumn:    "id",
		newRow:      func() objectReferenceRow { return &Key{} },
		rowID:       func(row objectReferenceRow) string { return row.(*Key).Id.String() },
		idValidator: idValidatorForPrefix(apid.PrefixKey),
	},
	namespaceschema.NamespaceKind: {
		table:                   NamespacesTable,
		idColumn:                "path",
		newRow:                  func() objectReferenceRow { return &Namespace{} },
		rowID:                   func(row objectReferenceRow) string { return row.(*Namespace).Path },
		idValidator:             namespaceschema.ValidatePath,
		namespacedNameCondition: namespaceNamespacedNameCondition,
	},
	ratelimitschema.RateLimitKind: {
		table:       RateLimitsTable,
		idColumn:    "id",
		newRow:      func() objectReferenceRow { return &RateLimit{} },
		rowID:       func(row objectReferenceRow) string { return row.(*RateLimit).Id.String() },
		idValidator: idValidatorForPrefix(apid.PrefixRateLimit),
	},
}

func idValidatorForPrefix(prefix apid.Prefix) func(string) error {
	return func(value string) error {
		id, err := apid.Parse(value)
		if err != nil {
			return err
		}
		return id.ValidatePrefix(prefix)
	}
}

func namespaceNamespacedNameCondition(reference meta.ObjectReference) (sq.Sqlizer, error) {
	path, err := namespaceschema.PathFromMetadata(meta.ObjectMeta{
		Name:      reference.Name,
		Namespace: reference.Namespace,
	})
	if err != nil {
		return nil, err
	}
	return sq.Eq{"path": path}, nil
}

func defaultNamespacedNameCondition(reference meta.ObjectReference) (sq.Sqlizer, error) {
	return sq.Eq{
		"namespace": reference.Namespace,
		"name":      reference.Name,
	}, nil
}

func validateObjectReference(
	reference meta.ObjectReference,
	target objectReferenceTarget,
) error {
	if err := meta.ValidateObjectReferenceWithOptions(
		reference,
		meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       reference.Kind,
			IDValidator:        target.idValidator,
			NamespaceValidator: namespaceschema.ValidatePath,
		},
		nil,
	); err != nil {
		return err
	}

	if (reference.Name == "") != (reference.Namespace == "") {
		return fmt.Errorf("namespace and name must be supplied together")
	}
	if reference.Generation != 0 && !target.allowGeneration {
		return fmt.Errorf("generation does not apply to %s references", reference.Kind)
	}
	return nil
}

func (s *service) lookupObjectReference(
	ctx context.Context,
	target objectReferenceTarget,
	condition sq.Sqlizer,
) (objectReferenceRow, error) {
	result := target.newRow()
	err := s.sq.
		Select(result.cols()...).
		From(target.table).
		Where(condition).
		Where(sq.Eq{"deleted_at": nil}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}

// resolveObjectReference is the single persistence-level reference resolver.
// It maps a supported kind to a trusted table definition and applies the same
// live-row identity rules to every resource. The generation field is not part
// of database identity; core uses it when hydrating versioned resources.
func (s *service) resolveObjectReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (objectReferenceRow, error) {
	target, ok := objectReferenceTargets[reference.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported kind %q", ErrInvalidReference, reference.Kind)
	}
	if err := validateObjectReference(reference, target); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReference, err)
	}

	var byID objectReferenceRow
	var err error
	if reference.HasID() {
		byID, err = s.lookupObjectReference(
			ctx,
			target,
			sq.Eq{target.idColumn: reference.ID},
		)
		if err != nil {
			return nil, err
		}
		if !reference.HasNamespacedName() {
			return byID, nil
		}
	}

	namespacedNameCondition := target.namespacedNameCondition
	if namespacedNameCondition == nil {
		namespacedNameCondition = defaultNamespacedNameCondition
	}
	condition, err := namespacedNameCondition(reference)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReference, err)
	}
	byName, err := s.lookupObjectReference(ctx, target, condition)
	if err != nil {
		return nil, err
	}

	if byID != nil && target.rowID(byID) != target.rowID(byName) {
		return nil, fmt.Errorf(
			"%w: id %q does not match %q/%q",
			ErrInvalidReference,
			reference.ID,
			reference.Namespace,
			reference.Name,
		)
	}
	return byName, nil
}

func validateResolvedReferenceKind(reference meta.ObjectReference, expected meta.Kind) error {
	if reference.Kind != expected {
		return fmt.Errorf(
			"%w: expected kind %q, got %q",
			ErrInvalidReference,
			expected,
			reference.Kind,
		)
	}
	return nil
}

func resolveObjectReferenceAs[T objectReferenceRow](
	ctx context.Context,
	s *service,
	reference meta.ObjectReference,
	expectedKind meta.Kind,
) (T, error) {
	var zero T
	if err := validateResolvedReferenceKind(reference, expectedKind); err != nil {
		return zero, err
	}
	value, err := s.resolveObjectReference(ctx, reference)
	if err != nil {
		return zero, err
	}
	result, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf(
			"resolved %s reference to unexpected database type %T: %w",
			reference.Kind,
			value,
			ErrViolation,
		)
	}
	return result, nil
}

func (s *service) ResolveActorReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*Actor, error) {
	return resolveObjectReferenceAs[*Actor](ctx, s, reference, actorschema.ActorKind)
}

func (s *service) ResolveConnectionReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*Connection, error) {
	return resolveObjectReferenceAs[*Connection](ctx, s, reference, connectionschema.ConnectionKind)
}

func (s *service) ResolveConnectorReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*Connector, error) {
	return resolveObjectReferenceAs[*Connector](ctx, s, reference, connectorschema.ConnectorKind)
}

func (s *service) ResolveKeyReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*Key, error) {
	return resolveObjectReferenceAs[*Key](ctx, s, reference, keyschema.KeyKind)
}

func (s *service) ResolveNamespaceReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*Namespace, error) {
	return resolveObjectReferenceAs[*Namespace](ctx, s, reference, namespaceschema.NamespaceKind)
}

func (s *service) ResolveRateLimitReference(
	ctx context.Context,
	reference meta.ObjectReference,
) (*RateLimit, error) {
	return resolveObjectReferenceAs[*RateLimit](ctx, s, reference, ratelimitschema.RateLimitKind)
}
