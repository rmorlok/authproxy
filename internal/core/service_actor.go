package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func (s *service) GetActor(
	ctx context.Context,
	id apid.ID,
) (iface.Actor, error) {
	actor, err := s.db.GetActor(ctx, id)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

func (s *service) GetActorByExternalId(
	ctx context.Context,
	namespace string,
	externalID string,
) (iface.Actor, error) {
	actor, err := s.db.GetActorByExternalId(ctx, namespace, externalID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

func (s *service) encryptActorSigningKey(
	ctx context.Context,
	namespace string,
	key *keyschema.SigningKey,
) (*encfield.EncryptedField, error) {
	if key == nil {
		return nil, nil
	}

	data, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("marshal actor signing key: %w", err)
	}

	encrypted, err := s.encrypt.EncryptStringForNamespace(
		ctx,
		namespace,
		string(data),
	)
	if err != nil {
		return nil, fmt.Errorf("encrypt actor signing key: %w", err)
	}

	return &encrypted, nil
}

func (s *service) CreateActor(
	ctx context.Context,
	resource *actorschema.Actor,
) (iface.Actor, error) {
	if resource == nil {
		return nil, errors.New("actor cannot be nil")
	}
	if err := resource.ValidateFor(
		meta.ValidationModeCreate,
		nil, // validation context
	); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	id := apid.New(apid.PrefixActor)
	normalized := resource.ApplyCreateDefaults(id)
	encryptedKey, err := s.encryptActorSigningKey(
		ctx,
		normalized.Metadata.Namespace,
		normalized.Spec.SigningKey,
	)
	if err != nil {
		return nil, err
	}

	actor, err := databaseActorFromResource(normalized, id, encryptedKey)
	if err != nil {
		return nil, err
	}

	if err := s.db.CreateActor(ctx, actor); err != nil {
		return nil, mapDatabaseError(err)
	}
	return s.GetActor(ctx, id)
}

// UpdateActor applies a presence-aware patch while preserving encrypted
// signing material when signingKey is omitted. Explicit null removes it.
func (s *service) UpdateActor(
	ctx context.Context,
	id apid.ID,
	patch *actorschema.ActorPatch,
) (iface.Actor, error) {
	if patch == nil {
		return nil, errors.New("actor patch cannot be nil")
	}

	existing, err := s.db.GetActor(ctx, id)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	before := actorResourceFromDatabase(*existing)
	desired, err := patch.ApplyTo(before, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	encryptedKey := existing.EncryptedKey
	if patch.Spec.HasSigningKey() {
		encryptedKey, err = s.encryptActorSigningKey(
			ctx,
			desired.Metadata.Namespace,
			patch.Spec.SigningKey,
		)
		if err != nil {
			return nil, err
		}
	}

	// Signing keys are write-only. Persistence validation and conversion see
	// only safe desired state plus the separately encrypted value.
	desired.Spec.SigningKey = nil
	desired.Status = &actorschema.ActorStatus{
		SigningKeyConfigured: encryptedKey != nil && !encryptedKey.IsZero(),
	}

	if err := desired.ValidateFor(
		meta.ValidationModePersistence,
		nil, // validation context
	); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}

	if before.Metadata.Name != desired.Metadata.Name {
		if _, err := s.db.UpdateActorName(
			ctx,
			id,
			desired.Metadata.Name,
		); err != nil {
			return nil, mapDatabaseError(err)
		}
	}

	actor, err := databaseActorFromResource(desired, id, encryptedKey)
	if err != nil {
		return nil, err
	}

	updated, err := s.db.UpsertActor(ctx, actor)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*updated, s), nil
}

func (s *service) DeleteActor(ctx context.Context, id apid.ID) error {
	return mapDatabaseError(s.db.DeleteActor(ctx, id))
}

func (s *service) PutActorLabels(
	ctx context.Context,
	id apid.ID,
	labels map[string]string,
) (iface.Actor, error) {
	actor, err := s.db.PutActorLabels(ctx, id, labels)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

func (s *service) DeleteActorLabels(
	ctx context.Context,
	id apid.ID,
	keys []string,
) (iface.Actor, error) {
	actor, err := s.db.DeleteActorLabels(ctx, id, keys)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

func (s *service) PutActorAnnotations(
	ctx context.Context,
	id apid.ID,
	annotations map[string]string,
) (iface.Actor, error) {
	actor, err := s.db.PutActorAnnotations(ctx, id, annotations)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

func (s *service) DeleteActorAnnotations(
	ctx context.Context,
	id apid.ID,
	keys []string,
) (iface.Actor, error) {
	actor, err := s.db.DeleteActorAnnotations(ctx, id, keys)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	return wrapActor(*actor, s), nil
}

type listActorWrapper struct {
	l database.ListActorsBuilder
	e database.ListActorsExecutor
	s *service
}

func (l *listActorWrapper) convertPageResult(
	result pagination.PageResult[*database.Actor],
) pagination.PageResult[iface.Actor] {
	if result.Error != nil {
		return pagination.PageResult[iface.Actor]{Error: result.Error}
	}

	actors := make([]iface.Actor, 0, len(result.Results))
	for _, actor := range result.Results {
		actors = append(actors, wrapActor(*actor, l.s))
	}

	return pagination.PageResult[iface.Actor]{
		Results: actors,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listActorWrapper) executor() database.ListActorsExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listActorWrapper) FetchPage(
	ctx context.Context,
) pagination.PageResult[iface.Actor] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listActorWrapper) Enumerate(
	ctx context.Context,
	callback pagination.EnumerateCallback[iface.Actor],
) error {
	return l.
		executor().
		Enumerate(ctx, func(result pagination.PageResult[*database.Actor]) (pagination.KeepGoing, error) {
			return callback(l.convertPageResult(result))
		})
}

func (l *listActorWrapper) ForExternalId(
	externalID string,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.ForExternalId(externalID), s: l.s}
}

func (l *listActorWrapper) ForName(
	name common.ResourceName,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.ForName(name), s: l.s}
}

func (l *listActorWrapper) ForNamespaceMatcher(
	matcher string,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listActorWrapper) ForNamespaceMatchers(
	matchers []string,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listActorWrapper) Limit(limit int32) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.Limit(limit), s: l.s}
}

func (l *listActorWrapper) OrderBy(
	field database.ActorOrderByField,
	order pagination.OrderBy,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.OrderBy(field, order), s: l.s}
}

func (l *listActorWrapper) IncludeDeleted() iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.IncludeDeleted(), s: l.s}
}

func (l *listActorWrapper) ForLabelSelector(
	selector string,
) iface.ListActorsBuilder {
	return &listActorWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (s *service) ListActorsBuilder() iface.ListActorsBuilder {
	return &listActorWrapper{l: s.db.ListActorsBuilder(), s: s}
}

func (s *service) ListActorsFromCursor(
	ctx context.Context,
	cursor string,
) (iface.ListActorsExecutor, error) {
	executor, err := s.db.ListActorsFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}
	return &listActorWrapper{e: executor, s: s}, nil
}

var _ iface.ListActorsBuilder = (*listActorWrapper)(nil)
