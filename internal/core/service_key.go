package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

func (s *service) GetKey(ctx context.Context, id apid.ID) (iface.Key, error) {
	ek, err := s.db.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return wrapKey(*ek, s), nil
}

func (s *service) CreateKey(ctx context.Context, namespace string, keyData *cfgschema.KeyData, labels map[string]string) (iface.Key, error) {
	ek := &database.Key{
		Id:        apid.New(apid.PrefixKey),
		Namespace: namespace,
		State:     database.KeyStateActive,
		Labels:    database.Labels(labels),
	}

	if keyData == nil {
		return nil, errors.New("key data cannot be nil")
	}

	keyDataBytes, err := json.Marshal(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key data to JSON: %w", err)
	}

	ef, err := s.encrypt.EncryptKeyForNamespace(ctx, namespace, keyDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key data: %w", err)
	}
	ek.EncryptedKeyData = &ef

	err = s.db.CreateKey(ctx, ek)
	if err != nil {
		return nil, err
	}

	// Enqueue immediate DEK generation so the new key can be used for
	// encryption without waiting for the next cron cycle. The follow-up sync
	// keeps wrapping metadata and namespace targets reconciled.
	encrypt.EnqueueGenerateDataEncryptionKeysToDatabase(ctx, s.ac, s.logger)
	encrypt.EnqueueForceSyncKeysToDatabase(ctx, s.r, s.ac, s.logger)

	return wrapKey(*ek, s), nil
}

func (s *service) GetKeyData(ctx context.Context, id apid.ID) (*cfgschema.KeyData, error) {
	ek, err := s.db.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ek.EncryptedKeyData == nil || ek.EncryptedKeyData.IsZero() {
		return nil, errors.New("key data is not configured")
	}

	keyDataBytes, err := s.encrypt.Decrypt(ctx, *ek.EncryptedKeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key data: %w", err)
	}

	var keyData cfgschema.KeyData
	if err := json.Unmarshal(keyDataBytes, &keyData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key data: %w", err)
	}

	return &keyData, nil
}

func (s *service) UpdateKeyData(ctx context.Context, id apid.ID, keyData *cfgschema.KeyData) (iface.Key, error) {
	if keyData == nil {
		return nil, errors.New("key data cannot be nil")
	}

	ek, err := s.db.GetKey(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	keyDataBytes, err := json.Marshal(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key data to JSON: %w", err)
	}

	ef, err := s.encrypt.EncryptKeyForNamespace(ctx, ek.Namespace, keyDataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key data: %w", err)
	}

	updated, err := s.db.UpdateKey(ctx, id, map[string]interface{}{
		"encrypted_key_data": ef,
	})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// A provider config replacement may change how DEKs are generated or
	// wrapped, so reconcile immediately rather than waiting for the cron.
	encrypt.EnqueueGenerateDataEncryptionKeysToDatabase(ctx, s.ac, s.logger)
	encrypt.EnqueueForceSyncKeysToDatabase(ctx, s.r, s.ac, s.logger)

	return wrapKey(*updated, s), nil
}

func (s *service) DeleteKey(ctx context.Context, id apid.ID) error {
	err := s.db.DeleteKey(ctx, id)
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

func (s *service) SetKeyState(ctx context.Context, id apid.ID, state database.KeyState) error {
	err := s.db.SetKeyState(ctx, id, state)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *service) UpdateKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.Key, error) {
	ek, err := s.db.UpdateKeyLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

func (s *service) PutKeyLabels(ctx context.Context, id apid.ID, labels map[string]string) (iface.Key, error) {
	ek, err := s.db.PutKeyLabels(ctx, id, labels)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

func (s *service) DeleteKeyLabels(ctx context.Context, id apid.ID, keys []string) (iface.Key, error) {
	ek, err := s.db.DeleteKeyLabels(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

func (s *service) UpdateKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.Key, error) {
	ek, err := s.db.UpdateKeyAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

func (s *service) PutKeyAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (iface.Key, error) {
	ek, err := s.db.PutKeyAnnotations(ctx, id, annotations)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

func (s *service) DeleteKeyAnnotations(ctx context.Context, id apid.ID, keys []string) (iface.Key, error) {
	ek, err := s.db.DeleteKeyAnnotations(ctx, id, keys)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return wrapKey(*ek, s), nil
}

type listKeyWrapper struct {
	l database.ListKeysBuilder
	e database.ListKeysExecutor
	s *service
}

func (l *listKeyWrapper) convertPageResult(result pagination.PageResult[database.Key]) pagination.PageResult[iface.Key] {
	if result.Error != nil {
		return pagination.PageResult[iface.Key]{Error: result.Error}
	}

	keys := make([]iface.Key, 0, len(result.Results))
	for _, r := range result.Results {
		keys = append(keys, wrapKey(r, l.s))
	}

	return pagination.PageResult[iface.Key]{
		Results: keys,
		Error:   result.Error,
		HasMore: result.HasMore,
		Cursor:  result.Cursor,
	}
}

func (l *listKeyWrapper) executor() database.ListKeysExecutor {
	if l.e != nil {
		return l.e
	}
	return l.l
}

func (l *listKeyWrapper) FetchPage(ctx context.Context) pagination.PageResult[iface.Key] {
	return l.convertPageResult(l.executor().FetchPage(ctx))
}

func (l *listKeyWrapper) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[iface.Key]) error {
	return l.executor().Enumerate(ctx, func(result pagination.PageResult[database.Key]) (keepGoing pagination.KeepGoing, err error) {
		return callback(l.convertPageResult(result))
	})
}

func (l *listKeyWrapper) Limit(lim int32) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.Limit(lim), s: l.s}
}

func (l *listKeyWrapper) ForNamespaceMatcher(matcher string) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.ForNamespaceMatcher(matcher), s: l.s}
}

func (l *listKeyWrapper) ForNamespaceMatchers(matchers []string) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.ForNamespaceMatchers(matchers), s: l.s}
}

func (l *listKeyWrapper) ForState(state database.KeyState) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.ForState(state), s: l.s}
}

func (l *listKeyWrapper) OrderBy(f database.KeyOrderByField, o pagination.OrderBy) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.OrderBy(f, o), s: l.s}
}

func (l *listKeyWrapper) IncludeDeleted() iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.IncludeDeleted(), s: l.s}
}

func (l *listKeyWrapper) ForLabelSelector(selector string) iface.ListKeysBuilder {
	return &listKeyWrapper{l: l.l.ForLabelSelector(selector), s: l.s}
}

func (s *service) ListKeysBuilder() iface.ListKeysBuilder {
	return &listKeyWrapper{
		l: s.db.ListKeysBuilder(),
		s: s,
	}
}

func (s *service) ListKeysFromCursor(ctx context.Context, cursor string) (iface.ListKeysExecutor, error) {
	e, err := s.db.ListKeysFromCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}

	return &listKeyWrapper{
		e: e,
		s: s,
	}, nil
}

var _ iface.ListKeysBuilder = (*listKeyWrapper)(nil)
