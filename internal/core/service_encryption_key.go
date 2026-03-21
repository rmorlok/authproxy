package core

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func (s *service) GetEncryptionKey(ctx context.Context, id apid.ID) (iface.EncryptionKey, error) {
	ek, err := s.db.GetEncryptionKey(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) CreateEncryptionKey(ctx context.Context, namespace string, keyData *cfgschema.KeyData, labels map[string]string) (iface.EncryptionKey, error) {
	ek := &database.EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: namespace,
		State:     database.EncryptionKeyStateActive,
		Labels:    database.Labels(labels),
	}

	if keyData == nil {
		return nil, errors.New("key data cannot be nil")
	}

	keyDataBytes, err := json.Marshal(keyData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal key data to JSON")
	}

	ef, err := s.encrypt.EncryptStringGlobal(ctx, string(keyDataBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to encrypt key data")
	}
	ek.EncryptedKeyData = &ef

	err = s.db.CreateEncryptionKey(ctx, ek)
	if err != nil {
		return nil, err
	}

	// Enqueue an immediate key sync task so the new key's versions are available
	// in the database without waiting for the next cron cycle.
	encrypt.EnqueueForceSyncKeysToDatabase(ctx, s.r, s.ac, s.logger)

	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) DeleteEncryptionKey(ctx context.Context, id apid.ID) error {
	err := s.db.DeleteEncryptionKey(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		if errors.Is(err, database.ErrProtected) {
			return err
		}
		return err
	}
	return nil
}

func (s *service) SetEncryptionKeyState(ctx context.Context, id apid.ID, state database.EncryptionKeyState) error {
	err := s.db.SetEncryptionKeyState(ctx, id, state)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *service) UpdateEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.EncryptionKey, error) {
	ek, err := s.db.UpdateEncryptionKeyLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) PutEncryptionKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.EncryptionKey, error) {
	ek, err := s.db.PutEncryptionKeyLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) DeleteEncryptionKeyLabels(ctx context.Context, id apid.ID, keys []string) (iface.EncryptionKey, error) {
	ek, err := s.db.DeleteEncryptionKeyLabels(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) UpdateEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.EncryptionKey, error) {
	ek, err := s.db.UpdateEncryptionKeyAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) PutEncryptionKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.EncryptionKey, error) {
	ek, err := s.db.PutEncryptionKeyAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

func (s *service) DeleteEncryptionKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (iface.EncryptionKey, error) {
	ek, err := s.db.DeleteEncryptionKeyAnnotations(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapEncryptionKey(*ek, s), nil
}

type listEncryptionKeyWrapper struct {
	l database.ListEncryptionKeysBuilder
	e database.ListEncryptionKeysExecutor
	s *service
}

func (l *listEncryptionKeyWrapper) convertPageResult(result pagination.PageResult[database.EncryptionKey]) pagination.PageResult[iface.EncryptionKey] {
	if result.Error != nil {
		return pagination.PageResult[iface.EncryptionKey]{Error: result.Error}
	}

	keys := make([]iface.EncryptionKey, 0, len(result.Results))
	for _, r := range result.Results {
		keys = append(keys, wrapEncryptionKey(r, l.s))
	}

	return pagination.PageResult[iface.EncryptionKey]{
		Results: keys,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listEncryptionKeyWrapper) executor() database.ListEncryptionKeysExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listEncryptionKeyWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.EncryptionKey] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listEncryptionKeyWrapper) Enumerate(ctx context.Context, callback func(pagination.PageResult[iface.EncryptionKey]) (keepGoing bool, err error)) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.EncryptionKey]) (keepGoing bool, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listEncryptionKeyWrapper) Limit(lim int32) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.Limit(lim), s: l.s}
}

func (l *listEncryptionKeyWrapper) ForNamespaceMatcher(matcher string) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listEncryptionKeyWrapper) ForNamespaceMatchers(matchers []string) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listEncryptionKeyWrapper) ForState(state database.EncryptionKeyState) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.ForState(state), s: l.s}
}

func (l *listEncryptionKeyWrapper) OrderBy(f database.EncryptionKeyOrderByField, o pagination.OrderBy) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.OrderBy(f, o), s: l.s}
}

func (l *listEncryptionKeyWrapper) IncludeDeleted() iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.IncludeDeleted(), s: l.s}
}

func (l *listEncryptionKeyWrapper) ForLabelSelector(selector string) iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (s *service) ListEncryptionKeysBuilder() iface.ListEncryptionKeysBuilder {
	return &listEncryptionKeyWrapper{
		l: s.db.ListEncryptionKeysBuilder(),
		s: s,
	}
}

func (s *service) ListEncryptionKeysFromCursor(ctx context.Context, cursor string) (iface.ListEncryptionKeysExecutor, error) {
	e, err := s.db.ListEncryptionKeysFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listEncryptionKeyWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListEncryptionKeysBuilder = (*listEncryptionKeyWrapper)(nil)
