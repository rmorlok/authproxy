package encrypt

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

const (
	TaskTypeSyncKeysToDatabase = "encrypt:sync_keys_to_database"
)

func NewSyncKeysToDatabaseTask() *asynq.Task {
	return asynq.NewTask(TaskTypeSyncKeysToDatabase, nil)
}

func (h *EncryptServiceTaskHandler) syncKeyDataDatabase(
	ctx context.Context,
	scope string,
	isCurrent bool,
	kd *config.KeyData,
) error {
	vers, err := kd.ListVersions(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s key versions", scope)
	}

	// Check if we already have a record for this provider+providerID+providerVersion
	existing, err := h.db.ListEncryptionKeyVersionsForScope(ctx, scope)
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
			maxVersion, err := h.db.GetMaxOrderedVersionForScope(ctx, scope)
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
				IsCurrent:       ver.IsCurrent && isCurrent,
			}

			if ver.IsCurrent && isCurrent {
				if err := h.db.ClearCurrentFlagForScope(ctx, scope); err != nil {
					return errors.Wrapf(err, "failed to clear current flag for scope %s", scope)
				}
			}

			if err := h.db.CreateEncryptionKeyVersion(ctx, ekv); err != nil {
				return errors.Wrapf(err, "failed to create encryption key version for index %d for scope %s", i, scope)
			}

			found = ekv
		} else if (ver.IsCurrent && isCurrent) && !found.IsCurrent {
			// This key should be current but isn't marked as such
			if err := h.db.ClearCurrentFlagForScope(ctx, scope); err != nil {
				return errors.Wrapf(err, "failed to clear current flag for scope %s", scope)
			}
			if err := h.db.SetEncryptionKeyVersionCurrentFlag(ctx, found.Id, true); err != nil {
				return errors.Wrapf(err, "failed to set current flag for scope %s for version %s", scope, found.Id)
			}
			found.IsCurrent = true
		}
	}

	// Remove any old versions that are no longer present
	for ekv := range unused.All() {
		if err := h.db.DeleteEncryptionKeyVersion(ctx, ekv.Id); err != nil {
			return errors.Wrapf(err, "failed to delete encryption key version %s for scope %s", ekv.Id, scope)
		}
	}

	return nil
}

func (h *EncryptServiceTaskHandler) handleSyncKeysToDatabase(ctx context.Context, task *asynq.Task) error {
	return h.SyncKeysToDatabase(ctx)
}

// SyncKeysToDatabase reads all configured keys and upserts encryption_key_versions records
func (h *EncryptServiceTaskHandler) SyncKeysToDatabase(ctx context.Context) error {
	h.logger.Info("syncing encryption keys to database")
	defer h.logger.Info("syncing encryption keys to datbase complete")

	m := apredis.NewMutex(
		h.redis,
		"encrypt:sync_keys",
		apredis.MutexOptionLockFor(30*time.Second),
		apredis.MutexOptionRetryFor(31*time.Second),
		apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)
	err := m.Lock(context.Background())
	if err != nil {
		h.logger.Info("failed to establish redis lock")
		return errors.Wrap(err, "failed to establish lock for encryption key sync")
	}
	defer m.Unlock(context.Background())

	if h.cfg == nil || h.cfg.GetRoot() == nil {
		return errors.New("no configuration available")
	}

	sa := h.cfg.GetRoot().SystemAuth
	if sa.GlobalAESKey == nil {
		return errors.New("no global AES key configured")
	}

	err = h.syncKeyDataDatabase(ctx, "global", true, sa.GlobalAESKey)
	if err != nil {
		return errors.Wrap(err, "failed to sync global key data")
	}

	return nil
}
