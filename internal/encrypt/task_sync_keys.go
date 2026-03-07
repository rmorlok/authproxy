package encrypt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-faster/errors"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	iconfig "github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

const (
	TaskTypeSyncKeysToDatabase = "encrypt:sync_keys_to_database"
	sentinelKey                = "encrypt:last_key_sync_time"
	sentinelTTL                = 15 * time.Minute
)

func NewSyncKeysToDatabaseTask() *asynq.Task {
	return asynq.NewTask(TaskTypeSyncKeysToDatabase, nil)
}

// versionWithCurrent pairs a key version info with whether it belongs to the current (primary) key.
type versionWithCurrent struct {
	ver       config.KeyVersionInfo
	isCurrent bool
}

// syncKeyDataToDatabase reconciles all key versions for a scope against the database.
// It takes all versions from all key datas for the scope at once, so that versions from
// different key datas don't delete each other.
func syncKeyDataToDatabase(
	ctx context.Context,
	db database.DB,
	scope string,
	isCurrent bool,
	kd *config.KeyData,
) error {
	vers, err := kd.ListVersions(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s key versions", scope)
	}

	// Check if we already have a record for this provider+providerID+providerVersion
	existing, err := db.ListEncryptionKeyVersionsForScope(ctx, scope)
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

		shouldBeCurrent := ver.IsCurrent && isCurrent

		if found == nil {
			// Create a new record
			maxVersion, err := db.GetMaxOrderedVersionForScope(ctx, scope)
			if err != nil {
				return errors.Wrap(err, "failed to get max ordered version")
			}

			ekv := &database.EncryptionKeyVersion{
				Id:              apid.New(apid.PrefixEncryptionKeyVersion),
				Scope:           scope,
				Provider:        string(ver.Provider),
				ProviderID:      ver.ProviderID,
				ProviderVersion: ver.ProviderVersion,
				OrderedVersion:  maxVersion + 1,
				IsCurrent:       shouldBeCurrent,
			}

			if shouldBeCurrent {
				if err := db.ClearCurrentFlagForScope(ctx, scope); err != nil {
					return errors.Wrapf(err, "failed to clear current flag for scope %s", scope)
				}
			}

			if err := db.CreateEncryptionKeyVersion(ctx, ekv); err != nil {
				return errors.Wrapf(err, "failed to create encryption key version for index %d for scope %s", i, scope)
			}

			found = ekv
		} else if shouldBeCurrent && !found.IsCurrent {
			// This key should be current but isn't marked as such
			if err := db.ClearCurrentFlagForScope(ctx, scope); err != nil {
				return errors.Wrapf(err, "failed to clear current flag for scope %s", scope)
			}
			if err := db.SetEncryptionKeyVersionCurrentFlag(ctx, found.Id, true); err != nil {
				return errors.Wrapf(err, "failed to set current flag for scope %s for version %s", scope, found.Id)
			}
			found.IsCurrent = true
		}
	}

	// Remove any old versions that are no longer present
	for ekv := range unused.All() {
		if err := db.DeleteEncryptionKeyVersion(ctx, ekv.Id); err != nil {
			return errors.Wrapf(err, "failed to delete encryption key version %s for scope %s", ekv.Id, scope)
		}
	}

	return nil
}

// syncKeysToDatabase is the standalone function that syncs configured keys into the database.
// It can be called directly during startup without constructing a task handler.
// When redis is provided, it uses a sentinel key to rate-limit syncs.
func syncKeysToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	logger.Info("syncing encryption keys to database")
	defer logger.Info("syncing encryption keys to database complete")

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
			return errors.Wrap(err, "failed to establish lock for encryption key sync")
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

	err := syncKeyDataToDatabase(ctx, db, "global", true, sa.GlobalAESKey)
	if err != nil {
		return errors.Wrap(err, "failed to sync global key data")
	}

	// Set sentinel after successful sync
	if redis != nil {
		if setErr := redis.Set(ctx, sentinelKey, fmt.Sprintf("%d", time.Now().Unix()), sentinelTTL).Err(); setErr != nil {
			logger.Warn("failed to set key sync sentinel", "error", setErr)
		}
	}

	return nil
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
