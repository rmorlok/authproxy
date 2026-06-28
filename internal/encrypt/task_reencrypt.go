package encrypt

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const (
	TaskTypeReencryptAll = "encrypt:reencrypt_all"
)

func NewReencryptAllTask() *asynq.Task {
	return asynq.NewTask(TaskTypeReencryptAll, nil)
}

func (h *EncryptServiceTaskHandler) handleReencryptAll(ctx context.Context, task *asynq.Task) error {
	h.logger.Info("starting re-encryption of all encrypted data")

	if err := h.enc.SyncKeysFromDbToMemory(ctx); err != nil {
		h.logger.Warn("failed to sync keys before re-encryption, proceeding with cached state", "error", err)
	}

	var totalProcessed, totalSkipped, totalErrors int

	err := h.db.EnumerateFieldsRequiringReEncryption(ctx, func(targets []database.ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
		var updates []database.ReEncryptedFieldUpdate

		for _, target := range targets {
			newEF, reencryptErr := h.enc.ReEncryptField(ctx, target.EncryptedFieldValue, target.TargetDataEncryptionKeyId)
			if reencryptErr != nil {
				h.logger.Warn("failed to re-encrypt field, skipping",
					"table", target.Table,
					"field", target.FieldColumn,
					"target_dek", target.TargetDataEncryptionKeyId,
					"error", reencryptErr,
				)
				totalErrors++
				continue
			}

			if newEF.ID == target.EncryptedFieldValue.ID {
				totalSkipped++
				continue
			}

			updates = append(updates, database.ReEncryptedFieldUpdate{
				Table:            target.Table,
				PrimaryKeyCols:   target.PrimaryKeyCols,
				PrimaryKeyValues: target.PrimaryKeyValues,
				FieldColumn:      target.FieldColumn,
				NewValue:         newEF,
			})
			totalProcessed++
		}

		if len(updates) > 0 {
			if updateErr := h.db.BatchUpdateReEncryptedFields(ctx, updates); updateErr != nil {
				h.logger.Error("failed to batch update re-encrypted fields", "error", updateErr)
				totalErrors += len(updates)
				totalProcessed -= len(updates)
			}
		}

		return pagination.Continue, nil
	})

	if err != nil {
		return fmt.Errorf("re-encryption enumeration failed: %w", err)
	}

	if err := h.reencryptKeys(ctx, &totalProcessed, &totalSkipped, &totalErrors); err != nil {
		return fmt.Errorf("key re-encryption failed: %w", err)
	}

	h.logger.Info("re-encryption complete",
		"total_processed", totalProcessed,
		"total_skipped", totalSkipped,
		"total_errors", totalErrors,
	)

	if totalErrors > 0 {
		return fmt.Errorf("re-encryption completed with %d errors", totalErrors)
	}

	return nil
}

func (h *EncryptServiceTaskHandler) reencryptKeys(ctx context.Context, totalProcessed, totalSkipped, totalErrors *int) error {
	now := apctx.GetClock(ctx).Now()

	return h.db.ListKeysBuilder().
		OrderBy(database.KeyOrderByCreatedAt, pagination.OrderByAsc).
		Enumerate(ctx, func(page pagination.PageResult[database.Key]) (pagination.KeepGoing, error) {
			if page.Error != nil {
				return pagination.Stop, page.Error
			}

			for _, key := range page.Results {
				if key.EncryptedKeyData == nil || key.EncryptedKeyData.IsZero() {
					(*totalSkipped)++
					continue
				}

				targetDEKID, ok, err := h.keyTargetDataEncryptionKeyID(ctx, key)
				if err != nil {
					h.logger.Warn("failed to resolve key re-encryption target, skipping",
						"key_id", key.Id,
						"namespace", key.Namespace,
						"error", err,
					)
					(*totalErrors)++
					continue
				}
				if !ok {
					h.logger.Warn("key parent namespace has no target data encryption key, skipping",
						"key_id", key.Id,
						"namespace", key.Namespace,
					)
					(*totalSkipped)++
					continue
				}

				if key.EncryptedKeyData.ID == targetDEKID {
					(*totalSkipped)++
					continue
				}

				newEF, err := h.enc.ReEncryptField(ctx, *key.EncryptedKeyData, targetDEKID)
				if err != nil {
					h.logger.Warn("failed to re-encrypt key data, skipping",
						"key_id", key.Id,
						"namespace", key.Namespace,
						"target_dek", targetDEKID,
						"error", err,
					)
					(*totalErrors)++
					continue
				}

				_, err = h.db.UpdateKey(ctx, key.Id, map[string]interface{}{
					"encrypted_key_data": newEF,
					"encrypted_at":       now,
				})
				if err != nil {
					h.logger.Warn("failed to update re-encrypted key data, skipping",
						"key_id", key.Id,
						"namespace", key.Namespace,
						"target_dek", targetDEKID,
						"error", err,
					)
					(*totalErrors)++
					continue
				}

				(*totalProcessed)++
			}

			return pagination.Continue, nil
		})
}

func (h *EncryptServiceTaskHandler) keyTargetDataEncryptionKeyID(ctx context.Context, key database.Key) (apid.ID, bool, error) {
	parentNamespace := namespace.ParentPath(key.Namespace)
	ns, err := h.db.GetNamespace(ctx, parentNamespace)
	if err != nil {
		return "", false, err
	}
	if ns.TargetDataEncryptionKeyId == nil {
		return "", false, nil
	}

	return *ns.TargetDataEncryptionKeyId, true, nil
}
