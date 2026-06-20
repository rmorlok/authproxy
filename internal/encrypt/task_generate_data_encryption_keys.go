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
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	iconfig "github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const (
	TaskTypeGenerateDataEncryptionKeys = "encrypt:generate_data_encryption_keys"
)

func NewGenerateDataEncryptionKeysTask() *asynq.Task {
	return asynq.NewTask(TaskTypeGenerateDataEncryptionKeys, nil)
}

func createDataEncryptionKey(
	ctx context.Context,
	db database.DB,
	encryptionKeyId apid.ID,
	kd *config.KeyData,
) (*database.DataEncryptionKey, error) {
	generated, err := kd.GenerateDataEncryptionKey(ctx)
	if err != nil {
		return nil, err
	}

	dek := &database.DataEncryptionKey{
		KeyId:           encryptionKeyId,
		Provider:        string(generated.Provider),
		ProviderID:      generated.ProviderID,
		ProviderVersion: generated.ProviderVersion,
		ProviderMetadata: database.DataEncryptionKeyProviderMetadata(
			generated.ProviderMetadata,
		),
		ProtectedData: &generated.ProtectedData,
		IsCurrent:     true,
	}
	if err := db.CreateDataEncryptionKey(ctx, dek); err != nil {
		return nil, err
	}

	return dek, nil
}

func ensureDataEncryptionKeyForKey(
	ctx context.Context,
	db database.DB,
	policy *config.DataEncryptionKeys,
	encryptionKeyId apid.ID,
	kd *config.KeyData,
) (bool, error) {
	if kd == nil {
		return false, errors.New("key data is nil")
	}

	current, err := db.GetCurrentDataEncryptionKeyForKey(ctx, encryptionKeyId)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return false, err
	}

	if current == nil {
		if !policy.ShouldEnsureCurrent() {
			return false, nil
		}
		if _, err := createDataEncryptionKey(ctx, db, encryptionKeyId, kd); err != nil {
			return false, err
		}
		return true, nil
	}

	if policy.ShouldRotate(apctx.GetClock(ctx).Now(), current.CreatedAt) {
		if _, err := createDataEncryptionKey(ctx, db, encryptionKeyId, kd); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func shouldManageDataEncryptionKeysForKey(key *database.Key) bool {
	if key == nil {
		return false
	}
	if key.State != database.KeyStateActive {
		return false
	}
	if key.Usage != database.KeyUsageDataEncryption {
		return false
	}

	switch key.MaterialType {
	case database.KeyMaterialTypeSymmetric, database.KeyMaterialTypeExternal:
		return true
	default:
		return false
	}
}

func generateDataEncryptionKeysToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	logger.Info("generating data encryption keys")
	defer logger.Info("generating data encryption keys complete")

	if redis != nil {
		m := apredis.NewMutex(
			redis,
			"encrypt:generate_deks",
			apredis.MutexOptionLockFor(30*time.Second),
			apredis.MutexOptionRetryFor(31*time.Second),
			apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
			apredis.MutexOptionDetailedLockMetadata(),
		)
		err := m.Lock(context.Background())
		if err != nil {
			return errors.Wrap(err, "failed to establish lock for data encryption key generation")
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

	var result *multierror.Error
	if err := ensureRootNamespaceUsesGlobalKey(ctx, db); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "failed to ensure root namespace uses global key"))
	}

	policy := sa.DataEncryptionKeys
	// key material id -> plaintext DEK for decrypting child key data.
	keyMaterialDataCache := make(map[apid.ID]config.KeyVersionInfo)

	generated, err := ensureDataEncryptionKeyForKey(ctx, db, policy, globalEncryptionKeyID, sa.GlobalAESKey)
	if err != nil {
		result = multierror.Append(result, errors.Wrap(err, "failed to reconcile data encryption key for global key"))
	} else {
		if generated {
			logger.Info("generated data encryption key", "key_id", globalEncryptionKeyID)
		}
	}
	err = cacheDataEncryptionKeysForKey(ctx, db, keyMaterialDataCache, globalEncryptionKeyID, sa.GlobalAESKey)
	if err != nil {
		result = multierror.Append(result, errors.Wrap(err, "failed to cache global data encryption keys"))
	}

	_, err = db.EnumerateKeysInDependencyOrder(ctx, func(keys []*database.Key, _ int) (keepGoing pagination.KeepGoing, err error) {
		for _, key := range keys {
			if key.Id == globalEncryptionKeyID {
				continue
			}
			if key.EncryptedKeyData == nil {
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

			if shouldManageDataEncryptionKeysForKey(key) {
				generated, err := ensureDataEncryptionKeyForKey(ctx, db, policy, key.Id, &keyData)
				if err != nil {
					result = multierror.Append(result, errors.Wrapf(err, "failed to reconcile data encryption key for key ID %s", key.Id))
					continue
				}
				if generated {
					logger.Info("generated data encryption key", "key_id", key.Id)
				}
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

	return result.ErrorOrNil()
}

func (h *EncryptServiceTaskHandler) handleGenerateDataEncryptionKeys(ctx context.Context, _ *asynq.Task) error {
	return generateDataEncryptionKeysToDatabase(ctx, h.cfg, h.db, h.logger, h.redis)
}

// GenerateDataEncryptionKeysToDatabase reconciles current DEKs for configured
// data-encryption keys without constructing the runtime encryption service.
func GenerateDataEncryptionKeysToDatabase(
	ctx context.Context,
	cfg iconfig.C,
	db database.DB,
	logger *slog.Logger,
	redis apredis.Client,
) error {
	return generateDataEncryptionKeysToDatabase(ctx, cfg, db, logger, redis)
}
