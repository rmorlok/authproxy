package encrypt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-faster/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	iconfig "github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const (
	TaskTypeSyncKeysToDatabase = "encrypt:sync_keys_to_database"
	sentinelKey                = "encrypt:last_key_sync_time"
	sentinelTTL                = 15 * time.Minute
)

func NewSyncKeysToDatabaseTask() *asynq.Task {
	return asynq.NewTask(TaskTypeSyncKeysToDatabase, nil)
}

func ensureRootNamespaceUsesGlobalKey(ctx context.Context, db database.DB) error {
	rootNamespace, err := db.GetNamespace(ctx, namespace.RootNamespace)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if rootNamespace.KeyId != nil {
		return nil
	}

	_, err = db.SetNamespaceKeyId(ctx, namespace.RootNamespace, &globalEncryptionKeyID)
	return err
}

// dataEncryptionKeyInfos converts a set of the database version of the DEK to the schema version.
func dataEncryptionKeyInfos(deks []*database.DataEncryptionKey) []config.DataEncryptionKeyInfo {
	infos := make([]config.DataEncryptionKeyInfo, 0, len(deks))
	for _, dek := range deks {
		infos = append(infos, config.DataEncryptionKeyInfo{
			ID:              string(dek.Id),
			EncryptionKeyID: string(dek.KeyId),
			Provider:        config.ProviderType(dek.Provider),
			ProviderID:      dek.ProviderID,
			ProviderVersion: dek.ProviderVersion,
			ProtectedData:   dek.ProtectedData,
			IsCurrent:       dek.IsCurrent,
		})
	}
	return infos
}

func cacheDataEncryptionKeysForKey(
	ctx context.Context,
	db database.DB,
	cache map[apid.ID]config.KeyVersionInfo,
	keyId apid.ID,
	kd *config.KeyData,
) error {
	deks, err := db.ListDataEncryptionKeysForKey(ctx, keyId)
	if err != nil {
		return errors.Wrap(err, "failed to list data encryption keys")
	}

	infos := dataEncryptionKeyInfos(deks)
	for i, dek := range deks {
		dekBytes, err := kd.UnwrapDataEncryptionKey(ctx, infos[i])
		if err != nil {
			return errors.Wrapf(err, "failed to unwrap data encryption key %s for key %s", dek.Id, keyId)
		}

		cache[dek.Id] = config.KeyVersionInfo{
			Provider:        config.ProviderType(dek.Provider),
			ProviderID:      dek.ProviderID,
			ProviderVersion: dek.ProviderVersion,
			Data:            dekBytes,
			IsCurrent:       dek.IsCurrent,
		}
	}

	return nil
}

func providerMetadataFromWrappingKey(info config.KeyWrappingKeyInfo) database.DataEncryptionKeyProviderMetadata {
	if len(info.Metadata) == 0 {
		return nil
	}

	metadata := make(database.DataEncryptionKeyProviderMetadata, len(info.Metadata))
	for k, v := range info.Metadata {
		metadata[k] = v
	}
	return metadata
}

func providerMetadataMatches(dekMetadata database.DataEncryptionKeyProviderMetadata, wrappingMetadata map[string]string) bool {
	if len(dekMetadata) != len(wrappingMetadata) {
		return false
	}
	for k, v := range wrappingMetadata {
		if dekMetadata[k] != v {
			return false
		}
	}
	return true
}

func dataEncryptionKeyNeedsRewrap(dek *database.DataEncryptionKey, current config.KeyWrappingKeyInfo) bool {
	if dek.Provider != string(current.Provider) {
		return true
	}
	if dek.ProviderID != current.ProviderID {
		return true
	}
	if dek.ProviderVersion != current.ProviderVersion {
		return true
	}
	return !providerMetadataMatches(dek.ProviderMetadata, current.Metadata)
}

func rewrapDataEncryptionKeysForKey(
	ctx context.Context,
	db database.DB,
	keyId apid.ID,
	kd *config.KeyData,
) error {
	current, err := kd.CurrentWrappingKey(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get current wrapping key for key %s", keyId)
	}

	var result *multierror.Error
	err = db.EnumerateDataEncryptionKeysForKey(ctx, keyId,
		func(deks []*database.DataEncryptionKey, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
			for _, dek := range deks {
				if !dataEncryptionKeyNeedsRewrap(dek, current) {
					continue
				}

				infos := dataEncryptionKeyInfos([]*database.DataEncryptionKey{dek})
				if len(infos) != 1 {
					result = multierror.Append(result, fmt.Errorf("failed to map data encryption key %s for key %s", dek.Id, keyId))
					continue
				}

				dekBytes, err := kd.UnwrapDataEncryptionKey(ctx, infos[0])
				if err != nil {
					result = multierror.Append(result, errors.Wrapf(err, "failed to unwrap data encryption key %s for key %s", dek.Id, keyId))
					continue
				}

				wrapped, err := kd.WrapDataEncryptionKey(ctx, dekBytes)
				if err != nil {
					result = multierror.Append(result, errors.Wrapf(err, "failed to rewrap data encryption key %s for key %s", dek.Id, keyId))
					continue
				}

				updated := *dek
				updated.Provider = string(wrapped.Provider)
				updated.ProviderID = wrapped.ProviderID
				updated.ProviderVersion = wrapped.ProviderVersion
				updated.ProviderMetadata = providerMetadataFromWrappingKey(current)
				updated.ProtectedData = &wrapped.ProtectedData

				if err := db.UpdateDataEncryptionKeyWrapping(ctx, &updated); err != nil {
					result = multierror.Append(result, errors.Wrapf(err, "failed to update data encryption key %s wrapping for key %s", dek.Id, keyId))
					continue
				}
			}

			return pagination.Continue, nil
		},
	)
	if err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func reconcileNamespaceEncryptionTargets(
	ctx context.Context,
	db database.DB,
	logger *slog.Logger,
) error {
	// effectiveDEK maps namespace path -> resolved target DEK ID, declared outside the callback
	// so it persists across pages (depth ordering ensures parents are processed first).
	effectiveDEK := make(map[string]apid.ID)

	return db.EnumerateNamespaceEncryptionTargets(ctx,
		func(targets []database.NamespaceEncryptionTarget, lastPage bool) ([]database.NamespaceTargetDataEncryptionKeyUpdate, pagination.KeepGoing, error) {
			var updates []database.NamespaceTargetDataEncryptionKeyUpdate

			for _, target := range targets {
				var resolvedDEKID apid.ID

				if target.KeyId != nil {
					// Namespace has its own key; look up its current DEK.
					currentDEK, err := db.GetCurrentDataEncryptionKeyForKey(ctx, *target.KeyId)
					if err != nil {
						logger.Warn("failed to get current data encryption key for namespace key",
							"namespace", target.Path,
							"key_id", *target.KeyId,
							"error", err,
						)
						continue
					}
					resolvedDEKID = currentDEK.Id
				} else {
					// Inherit from nearest ancestor with a resolved DEK.
					prefixes := namespace.SplitNamespacePathToPrefixes(target.Path)
					found := false
					for i := len(prefixes) - 2; i >= 0; i-- {
						if dekID, ok := effectiveDEK[prefixes[i]]; ok {
							resolvedDEKID = dekID
							found = true
							break
						}
					}

					if !found {
						// Fall back to the global key's current DEK.
						globalDEK, err := db.GetCurrentDataEncryptionKeyForKey(ctx, globalEncryptionKeyID)
						if err != nil {
							logger.Warn("failed to get current data encryption key for global key",
								"namespace", target.Path,
								"error", err,
							)
							continue
						}
						resolvedDEKID = globalDEK.Id
					}
				}

				effectiveDEK[target.Path] = resolvedDEKID

				// Only emit an update if the value actually changed
				if target.TargetDataEncryptionKeyId == nil || *target.TargetDataEncryptionKeyId != resolvedDEKID {
					updates = append(updates, database.NamespaceTargetDataEncryptionKeyUpdate{
						Path:                      target.Path,
						TargetDataEncryptionKeyId: resolvedDEKID,
					})
				}
			}

			return updates, pagination.Continue, nil
		},
	)
}

// syncKeysToDatabase is the standalone function that syncs key wrapping state into the database.
// It rewraps existing DEKs with their provider's current wrapping material and refreshes namespace DEK
// targets. It can be called directly during startup without constructing a task handler. When redis is
// provided, it uses a sentinel key to rate-limit syncs.
func syncKeysToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	logger.Info("syncing data encryption key wrapping to database")
	defer logger.Info("syncing data encryption key wrapping to database complete")

	if err := ensureRootNamespaceUsesGlobalKey(ctx, db); err != nil {
		return errors.Wrap(err, "failed to ensure root namespace uses global key")
	}

	if redis != nil {
		// Redis can be skipped in test cases
		val, err := redis.Get(ctx, sentinelKey).Result()
		if err == nil && val != "" {
			logger.Info("skipping key sync: recently synced")
			return nil
		}

		m := apredis.NewMutex(
			redis,
			"encrypt:sync_keys",
			apredis.MutexOptionLockFor(30*time.Second),
			apredis.MutexOptionRetryFor(31*time.Second),
			apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
			apredis.MutexOptionDetailedLockMetadata(),
		)
		err = m.Lock(context.Background())
		if err != nil {
			logger.Info("failed to establish redis lock")
			return errors.Wrap(err, "failed to establish lock for data encryption key wrapping sync")
		}
		defer m.Unlock(context.Background())
	}

	if cfg == nil || cfg.GetRoot() == nil {
		return errors.New("no configuration available")
	}

	sa := cfg.GetRoot().SystemAuth
	if sa.GlobalAESKey == nil {
		return errors.New("no global AES key configured")
	}

	// key material id -> plaintext DEK for decrypting child key data.
	keyMaterialDataCache := make(map[apid.ID]config.KeyVersionInfo)

	// Manually sync the global key first because other keys depend on it.
	err := rewrapDataEncryptionKeysForKey(ctx, db, globalEncryptionKeyID, sa.GlobalAESKey)
	if err != nil {
		return errors.Wrap(err, "failed to rewrap global data encryption keys")
	}
	err = cacheDataEncryptionKeysForKey(ctx, db, keyMaterialDataCache, globalEncryptionKeyID, sa.GlobalAESKey)
	if err != nil {
		return errors.Wrap(err, "failed to cache global data encryption keys")
	}

	var result *multierror.Error

	_, err = db.EnumerateKeysInDependencyOrder(ctx, func(keys []*database.Key, _ int) (keepGoing pagination.KeepGoing, err error) {
		for _, key := range keys {
			if key.EncryptedKeyData == nil {
				// Global key already synced
				continue
			}

			ef := key.EncryptedKeyData
			kvi, ok := keyMaterialDataCache[ef.ID]
			if !ok {
				result = multierror.Append(result, fmt.Errorf("key material info not found for key ID %s key material %s", key.Id, ef.ID))
				continue
			}

			decodedData, err := base64.StdEncoding.DecodeString(ef.Data)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to decode base64 string for key id %s", key.Id))
				continue
			}

			decryptedData, err := decryptWithKey(kvi.Data, decodedData)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to decrypt key id %s with key data from %s", key.Id, ef.ID))
				continue
			}

			var keyData config.KeyData
			err = json.Unmarshal(decryptedData, &keyData)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to unmarshal key data for key ID %s", key.Id))
				continue
			}

			err = rewrapDataEncryptionKeysForKey(ctx, db, key.Id, &keyData)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to rewrap data encryption keys for key ID %s", key.Id))
				continue
			}

			err = cacheDataEncryptionKeysForKey(ctx, db, keyMaterialDataCache, key.Id, &keyData)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to cache data encryption keys for key ID %s", key.Id))
				continue
			}
		}

		return pagination.Continue, nil
	})

	if err != nil {
		result = multierror.Append(result, err)
	}

	err = reconcileNamespaceEncryptionTargets(ctx, db, logger)
	if err != nil {
		result = multierror.Append(result, errors.Wrap(err, "failed to update namespace target data encryption keys"))
	}

	// Set sentinel after successful sync
	if redis != nil {
		now := apctx.GetClock(ctx).Now()
		if setErr := redis.Set(ctx, sentinelKey, fmt.Sprintf("%d", now.Unix()), sentinelTTL).Err(); setErr != nil {
			logger.Warn("failed to set key sync sentinel", "error", setErr)
		}
	}

	return result.ErrorOrNil()
}

// SyncKeysToDatabase is the exported standalone function for use by dependency_manager and tests.
func SyncKeysToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	return syncKeysToDatabase(ctx, cfg, db, logger, redis)
}

func (h *EncryptServiceTaskHandler) handleSyncKeysToDatabase(ctx context.Context, task *asynq.Task) error {
	return h.doSyncKeysToDatabase(ctx)
}

// doSyncKeysToDatabase delegates to the standalone function with the redis sentinel.
func (h *EncryptServiceTaskHandler) doSyncKeysToDatabase(ctx context.Context) error {
	return syncKeysToDatabase(ctx, h.cfg, h.db, h.logger, h.redis)
}

// EnqueueForceSyncKeysToDatabase clears the sync sentinel and enqueues a sync task immediately.
// The mutex inside the sync function prevents concurrent syncs.
func EnqueueForceSyncKeysToDatabase(ctx context.Context, redis apredis.Client, ac apasynq.Client, logger *slog.Logger) {
	redis.Del(ctx, sentinelKey)
	if _, err := ac.EnqueueContext(ctx, NewSyncKeysToDatabaseTask()); err != nil {
		logger.Warn("failed to enqueue force key sync task", "error", err)
	}
}
