package encrypt

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
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
			newEF, reencryptErr := h.enc.ReEncryptField(ctx, target.EncryptedFieldValue, target.TargetEncryptionKeyVersionId)
			if reencryptErr != nil {
				h.logger.Warn("failed to re-encrypt field, skipping",
					"table", target.Table,
					"field", target.FieldColumn,
					"target_ekv", target.TargetEncryptionKeyVersionId,
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

		return true, nil
	})

	if err != nil {
		return fmt.Errorf("re-encryption enumeration failed: %w", err)
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
