package encrypt

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
)

const (
	TaskTypeReencryptAll = "encrypt:reencrypt_all"
)

type ReencryptTaskHandler struct {
	db     database.DB
	enc    E
	logger *slog.Logger
}

func NewReencryptTaskHandler(db database.DB, enc E, logger *slog.Logger) *ReencryptTaskHandler {
	return &ReencryptTaskHandler{
		db:     db,
		enc:    enc,
		logger: logger,
	}
}

func NewReencryptAllTask() *asynq.Task {
	return asynq.NewTask(TaskTypeReencryptAll, nil)
}

func (h *ReencryptTaskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskTypeReencryptAll, h.handleReencryptAll)
}

func (h *ReencryptTaskHandler) handleReencryptAll(ctx context.Context, task *asynq.Task) error {
	h.logger.Info("starting re-encryption of all encrypted data")

	var totalProcessed, totalSkipped, totalErrors int

	// Re-encrypt OAuth2 tokens
	processed, skipped, errs := h.reencryptOAuth2Tokens(ctx)
	totalProcessed += processed
	totalSkipped += skipped
	totalErrors += errs
	h.logger.Info("re-encrypted oauth2 tokens", "processed", processed, "skipped", skipped, "errors", errs)

	// Re-encrypt actor encrypted keys
	processed, skipped, errs = h.reencryptActorKeys(ctx)
	totalProcessed += processed
	totalSkipped += skipped
	totalErrors += errs
	h.logger.Info("re-encrypted actor keys", "processed", processed, "skipped", skipped, "errors", errs)

	// Re-encrypt connector version definitions
	processed, skipped, errs = h.reencryptConnectorVersions(ctx)
	totalProcessed += processed
	totalSkipped += skipped
	totalErrors += errs
	h.logger.Info("re-encrypted connector versions", "processed", processed, "skipped", skipped, "errors", errs)

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

func (h *ReencryptTaskHandler) reencryptOAuth2Tokens(ctx context.Context) (processed, skipped, errs int) {
	err := h.db.EnumerateOAuth2Tokens(ctx, func(tokens []*database.OAuth2Token, lastPage bool) (stop bool, err error) {
		var updates []database.OAuth2TokenEncryptedFieldsUpdate

		for _, token := range tokens {
			accessAlready := h.enc.IsEncryptedWithPrimaryKey(token.EncryptedAccessToken)
			refreshAlready := h.enc.IsEncryptedWithPrimaryKey(token.EncryptedRefreshToken)

			if accessAlready && refreshAlready {
				skipped++
				continue
			}

			newAccess := token.EncryptedAccessToken
			newRefresh := token.EncryptedRefreshToken

			if !accessAlready && token.EncryptedAccessToken != "" {
				plain, decryptErr := h.enc.DecryptStringGlobal(ctx, token.EncryptedAccessToken)
				if decryptErr != nil {
					h.logger.Error("failed to decrypt access token", "id", token.Id, "error", decryptErr)
					errs++
					continue
				}
				newAccess, err = h.enc.EncryptStringGlobal(ctx, plain)
				if err != nil {
					h.logger.Error("failed to re-encrypt access token", "id", token.Id, "error", err)
					errs++
					continue
				}
			}

			if !refreshAlready && token.EncryptedRefreshToken != "" {
				plain, decryptErr := h.enc.DecryptStringGlobal(ctx, token.EncryptedRefreshToken)
				if decryptErr != nil {
					h.logger.Error("failed to decrypt refresh token", "id", token.Id, "error", decryptErr)
					errs++
					continue
				}
				newRefresh, err = h.enc.EncryptStringGlobal(ctx, plain)
				if err != nil {
					h.logger.Error("failed to re-encrypt refresh token", "id", token.Id, "error", err)
					errs++
					continue
				}
			}

			updates = append(updates, database.OAuth2TokenEncryptedFieldsUpdate{
				Id:                    token.Id,
				EncryptedAccessToken:  newAccess,
				EncryptedRefreshToken: newRefresh,
			})
			processed++
		}

		if len(updates) > 0 {
			if updateErr := h.db.BatchUpdateOAuth2TokenEncryptedFields(ctx, updates); updateErr != nil {
				h.logger.Error("failed to batch update oauth2 tokens", "error", updateErr)
				errs += len(updates)
				processed -= len(updates)
			}
		}

		return false, nil
	})
	if err != nil {
		h.logger.Error("failed to enumerate oauth2 tokens", "error", err)
		errs++
	}

	return
}

func (h *ReencryptTaskHandler) reencryptActorKeys(ctx context.Context) (processed, skipped, errs int) {
	err := h.db.EnumerateActorsWithEncryptedKey(ctx, func(actors []*database.Actor, lastPage bool) (stop bool, err error) {
		var updates []database.ActorEncryptedKeyUpdate

		for _, actor := range actors {
			if actor.EncryptedKey == nil {
				skipped++
				continue
			}

			encKey := *actor.EncryptedKey

			if h.enc.IsEncryptedWithPrimaryKey(encKey) {
				skipped++
				continue
			}

			plain, decryptErr := h.enc.DecryptStringGlobal(ctx, encKey)
			if decryptErr != nil {
				h.logger.Error("failed to decrypt actor key", "id", actor.Id, "error", decryptErr)
				errs++
				continue
			}

			newEnc, encryptErr := h.enc.EncryptStringGlobal(ctx, plain)
			if encryptErr != nil {
				h.logger.Error("failed to re-encrypt actor key", "id", actor.Id, "error", encryptErr)
				errs++
				continue
			}

			updates = append(updates, database.ActorEncryptedKeyUpdate{
				Id:           actor.Id,
				EncryptedKey: newEnc,
			})
			processed++
		}

		if len(updates) > 0 {
			if updateErr := h.db.BatchUpdateActorEncryptedKey(ctx, updates); updateErr != nil {
				h.logger.Error("failed to batch update actor keys", "error", updateErr)
				errs += len(updates)
				processed -= len(updates)
			}
		}

		return false, nil
	})
	if err != nil {
		h.logger.Error("failed to enumerate actors", "error", err)
		errs++
	}

	return
}

func (h *ReencryptTaskHandler) reencryptConnectorVersions(ctx context.Context) (processed, skipped, errs int) {
	err := h.db.EnumerateConnectorVersions(ctx, func(cvs []*database.ConnectorVersion, lastPage bool) (stop bool, err error) {
		var updates []database.ConnectorVersionEncryptedDefinitionUpdate

		for _, cv := range cvs {
			if h.enc.IsEncryptedWithPrimaryKey(cv.EncryptedDefinition) {
				skipped++
				continue
			}

			plain, decryptErr := h.enc.DecryptStringGlobal(ctx, cv.EncryptedDefinition)
			if decryptErr != nil {
				h.logger.Error("failed to decrypt connector definition", "id", cv.Id, "version", cv.Version, "error", decryptErr)
				errs++
				continue
			}

			newEnc, encryptErr := h.enc.EncryptStringGlobal(ctx, plain)
			if encryptErr != nil {
				h.logger.Error("failed to re-encrypt connector definition", "id", cv.Id, "version", cv.Version, "error", encryptErr)
				errs++
				continue
			}

			updates = append(updates, database.ConnectorVersionEncryptedDefinitionUpdate{
				Id:                  cv.Id,
				Version:             cv.Version,
				EncryptedDefinition: newEnc,
			})
			processed++
		}

		if len(updates) > 0 {
			if updateErr := h.db.BatchUpdateConnectorVersionEncryptedDefinition(ctx, updates); updateErr != nil {
				h.logger.Error("failed to batch update connector versions", "error", updateErr)
				errs += len(updates)
				processed -= len(updates)
			}
		}

		return false, nil
	})
	if err != nil {
		h.logger.Error("failed to enumerate connector versions", "error", err)
		errs++
	}

	return
}
