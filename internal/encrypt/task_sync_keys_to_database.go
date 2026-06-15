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
	"github.com/rmorlok/authproxy/internal/util"
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

// syncKeyVersionsForKeyToDatabase reconciles all key versions for an encryption key against the database.
// It takes all versions from all key datas for the encryption key at once, so that versions from
// different key datas don't delete each other.
func syncKeyVersionsForKeyToDatabase(
	ctx context.Context,
	db database.DB,
	cache map[apid.ID]config.KeyVersionInfo,
	encryptionKeyId apid.ID,
	kd *config.KeyData,
) error {
	var vers []config.KeyVersionInfo
	var err error
	if kd.RequiresDataEncryptionKeys() {
		deks, err := db.ListDataEncryptionKeysForKey(ctx, encryptionKeyId)
		if err != nil {
			return errors.Wrap(err, "failed to list data encryption keys")
		}

		// KMS-style providers consume already-generated DEK rows. The sync task
		// maps those rows into application-facing encryption_key_versions; it does
		// not create DEKs as a side effect.
		vers, err = kd.ListVersionsWithDataEncryptionKeys(ctx, dataEncryptionKeyInfos(deks))
		if err != nil {
			return errors.Wrapf(err, "failed to get %s key versions", encryptionKeyId)
		}
	} else {
		// Key data doesn't require DEKs, just list directly
		vers, err = kd.ListVersions(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get %s key versions", encryptionKeyId)
		}
	}

	existing, err := db.ListEncryptionKeyVersionsForKey(ctx, encryptionKeyId)
	if err != nil {
		return errors.Wrap(err, "failed to list existing encryption key versions")
	}
	unused := util.NewSetFrom(existing)

	for i, ver := range vers {
		var found *database.EncryptionKeyVersion
		for ekv := range unused.All() {
			if ekv.Provider == string(ver.Provider) &&
				ekv.ProviderID == ver.ProviderID &&
				ekv.ProviderVersion == ver.ProviderVersion {
				unused.Remove(ekv)
				found = ekv
				break
			}
		}

		if found == nil {
			// Create a new record
			maxVersion, err := db.GetMaxOrderedVersionForKey(ctx, encryptionKeyId)
			if err != nil {
				return errors.Wrap(err, "failed to get max ordered version")
			}

			ekv := &database.EncryptionKeyVersion{
				Id:              apid.New(apid.PrefixEncryptionKeyVersion),
				KeyId:           encryptionKeyId,
				Provider:        string(ver.Provider),
				ProviderID:      ver.ProviderID,
				ProviderVersion: ver.ProviderVersion,
				OrderedVersion:  maxVersion + 1,
				IsCurrent:       ver.IsCurrent,
			}

			// Cache the information
			cache[ekv.Id] = ver

			if ver.IsCurrent {
				if err := db.ClearCurrentFlagForKey(ctx, encryptionKeyId); err != nil {
					return errors.Wrapf(err, "failed to clear current flag for encryption key %s", encryptionKeyId)
				}
			}

			if err := db.CreateEncryptionKeyVersion(ctx, ekv); err != nil {
				return errors.Wrapf(err, "failed to create encryption key version for index %d for encryption key %s", i, encryptionKeyId)
			}

			found = ekv
		} else {
			// Cache the information
			cache[found.Id] = ver

			if ver.IsCurrent && !found.IsCurrent {
				// This key should be current but isn't marked as such
				if err := db.ClearCurrentFlagForKey(ctx, encryptionKeyId); err != nil {
					return errors.Wrapf(err, "failed to clear current flag for encryption key %s", encryptionKeyId)
				}
				if err := db.SetEncryptionKeyVersionCurrentFlag(ctx, found.Id, true); err != nil {
					return errors.Wrapf(err, "failed to set current flag for encryption key %s for version %s", encryptionKeyId, found.Id)
				}
				found.IsCurrent = true
			}
		}
	}

	// Remove any old versions that are no longer present
	for ekv := range unused.All() {
		if err := db.DeleteEncryptionKeyVersion(ctx, ekv.Id); err != nil {
			return errors.Wrapf(err, "failed to delete encryption key version %s for encryption key %s", ekv.Id, encryptionKeyId)
		}
	}

	return nil
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

// syncKeysVersionsToDatabase is the standalone function that syncs key versions into the database. It lists
// all keys and makes sure the version for those keys are accurate. It can be called directly during startup
// without constructing a task handler. When redis is provided, it uses a sentinel key to rate-limit syncs.
func syncKeysVersionsToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	logger.Info("syncing encryption key versions to database")
	defer logger.Info("syncing encryption key versions to database complete")

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
			return errors.Wrap(err, "failed to establish lock for encryption key version sync")
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

	// key material id -> data for that material. During the migration this can
	// contain both transitional ekv_ ids and canonical dek_ ids.
	keyVersionIdDataCache := make(map[apid.ID]config.KeyVersionInfo)

	// Manually sync the global key first because other keys depend on it
	err := syncKeyVersionsForKeyToDatabase(ctx, db, keyVersionIdDataCache, globalEncryptionKeyID, sa.GlobalAESKey)
	if err != nil {
		return errors.Wrap(err, "failed to sync global key data")
	}
	err = cacheDataEncryptionKeysForKey(ctx, db, keyVersionIdDataCache, globalEncryptionKeyID, sa.GlobalAESKey)
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
			kvi, ok := keyVersionIdDataCache[ef.ID]
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

			err = syncKeyVersionsForKeyToDatabase(ctx, db, keyVersionIdDataCache, key.Id, &keyData)
			if err != nil {
				result = multierror.Append(result, errors.Wrapf(err, "failed to sync key data for key ID %s", key.Id))
				continue
			}

			err = cacheDataEncryptionKeysForKey(ctx, db, keyVersionIdDataCache, key.Id, &keyData)
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
	return syncKeysVersionsToDatabase(ctx, cfg, db, logger, redis)
}

func (h *EncryptServiceTaskHandler) handleSyncKeysToDatabase(ctx context.Context, task *asynq.Task) error {
	return h.doSyncKeysToDatabase(ctx)
}

// doSyncKeysToDatabase delegates to the standalone function with the redis sentinel.
func (h *EncryptServiceTaskHandler) doSyncKeysToDatabase(ctx context.Context) error {
	return syncKeysVersionsToDatabase(ctx, h.cfg, h.db, h.logger, h.redis)
}

// EnqueueForceSyncKeysToDatabase clears the sync sentinel and enqueues a sync task immediately.
// The mutex inside the sync function prevents concurrent syncs.
func EnqueueForceSyncKeysToDatabase(ctx context.Context, redis apredis.Client, ac apasynq.Client, logger *slog.Logger) {
	redis.Del(ctx, sentinelKey)
	if _, err := ac.EnqueueContext(ctx, NewSyncKeysToDatabaseTask()); err != nil {
		logger.Warn("failed to enqueue force key sync task", "error", err)
	}
}
