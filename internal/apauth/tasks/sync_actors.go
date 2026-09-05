package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// SyncActorList synchronizes actors from ConfiguredActorsList configuration to the database.
func (s *service) SyncActorList(ctx context.Context) error {
	actors := s.cfg.GetRoot().SystemAuth.Actors
	if actors == nil {
		s.logger.Info("no actors configured, skipping sync")
		return nil
	}

	// Check if this is a ConfiguredActorsList (not external source)
	if _, ok := actors.InnerVal.(sconfig.ConfiguredActorsList); !ok {
		s.logger.Debug("actors is not a list type, skipping list sync")
		return nil
	}

	return s.syncConfiguredActors(ctx, actors.All(), LabelValueConfigList)
}

// SyncConfiguredActorsExternalSource synchronizes actors from ConfiguredActorsExternalSource configuration to the database.
// This function uses a distributed lock to prevent concurrent syncs across multiple workers.
func (s *service) SyncConfiguredActorsExternalSource(ctx context.Context) error {
	actors := s.cfg.GetRoot().SystemAuth.Actors
	if actors == nil {
		s.logger.Info("no actors configured, skipping sync")
		return nil
	}

	// Check if this is a ConfiguredActorsExternalSource
	if _, ok := actors.InnerVal.(*sconfig.ConfiguredActorsExternalSource); !ok {
		s.logger.Debug("actors is not an external source type, skipping external source sync")
		return nil
	}

	// Only acquire lock if Redis is available
	if s.redis != nil {
		m := apredis.NewMutex(
			s.redis,
			MutexKeySyncActorsExternalSource,
			apredis.MutexOptionLockFor(defaultSyncLockDuration),
			apredis.MutexOptionNoRetry(),
			apredis.MutexOptionDetailedLockMetadata(),
		)

		err := apredis.RunWithMutex(ctx, m, defaultSyncLockDuration, func(lockCtx context.Context) error {
			return s.syncConfiguredActors(lockCtx, actors.All(), LabelValuePublicKeyDir)
		})
		if err != nil {
			if apredis.MutexIsErrNotObtained(err) {
				s.logger.Info("another sync is in progress, skipping this run")
				return nil
			}
			return fmt.Errorf("failed to run synchronized actor sync: %w", err)
		}
		return nil
	}

	return s.syncConfiguredActors(ctx, actors.All(), LabelValuePublicKeyDir)
}

// syncConfiguredActors performs the actual sync of configured actors to the database.
func (s *service) syncConfiguredActors(ctx context.Context, actors []*actorschema.Actor, sourceLabel string) error {
	// Build a set of expected external IDs
	expectedExternalIds := make(map[string]bool)

	// Upsert each configured actor
	for i, actor := range actors {
		if actor == nil {
			return fmt.Errorf("configured actor %d is nil", i)
		}
		if err := actor.ValidateFor(
			meta.ValidationModeConfig,
			(&common.ValidationContext{Path: "$.systemAuth.actors"}).PushIndex(i),
		); err != nil {
			return fmt.Errorf("invalid configured actor: %w", err)
		}
		externalId := actor.Spec.ExternalId
		expectedExternalIds[externalId] = true

		// Serialize and encrypt the key
		var encryptedKey *encfield.EncryptedField
		if actor.Spec.SigningKey != nil {
			keyJson, err := json.Marshal(actor.Spec.SigningKey)
			if err != nil {
				return fmt.Errorf("failed to marshal key for actor %s: %w", externalId, err)
			}

			encrypted, err := s.encrypt.EncryptStringGlobal(ctx, string(keyJson))
			if err != nil {
				return fmt.Errorf("failed to encrypt key for actor %s: %w", externalId, err)
			}
			encryptedKey = &encrypted
		}

		// Create labels, starting with actor's configured labels
		labels := make(database.Labels)
		for k, v := range actor.Metadata.Labels {
			labels[k] = v
		}
		// Add the sync source label
		labels[LabelConfiguredActorSyncSource] = sourceLabel

		// Create actor data with labels and encrypted key
		actorData := &configuredActorData{
			namespace:    actor.Metadata.Namespace,
			externalId:   externalId,
			labels:       labels,
			annotations:  database.Annotations(actor.Metadata.Annotations),
			permissions:  actor.Spec.Permissions,
			encryptedKey: encryptedKey,
		}

		// Upsert the actor
		dbActor, err := s.db.UpsertActor(ctx, actorData)
		if err != nil {
			return fmt.Errorf("failed to upsert actor %s: %w", externalId, err)
		}
		if actor.Metadata.Name != "" && actor.Metadata.Name != dbActor.Name {
			if _, err := s.db.UpdateActorName(ctx, dbActor.Id, actor.Metadata.Name); err != nil {
				return fmt.Errorf("failed to update name for actor %s: %w", externalId, err)
			}
		}

		s.logger.Debug("synced configured actor", "external_id", externalId)
	}

	// Delete stale actors (those with the sync label but not in current config)
	err := s.db.ListActorsBuilder().
		ForLabelSelector(LabelConfiguredActorSyncSource).
		Enumerate(ctx, func(result pagination.PageResult[*database.Actor]) (keepGoing pagination.KeepGoing, err error) {
			for _, dbActor := range result.Results {
				// Only delete actors with matching source label that aren't in current config
				if dbActor.Labels[LabelConfiguredActorSyncSource] == sourceLabel && !expectedExternalIds[dbActor.ExternalId] {
					s.logger.Info("deleting stale configured actor", "external_id", dbActor.ExternalId)
					if err := s.db.DeleteActor(ctx, dbActor.Id); err != nil {
						return pagination.Stop, fmt.Errorf("failed to delete stale actor %s: %w", dbActor.ExternalId, err)
					}
				}
			}
			return pagination.Continue, nil
		})

	if err != nil {
		return fmt.Errorf("failed to enumerate and cleanup stale actors: %w", err)
	}

	s.logger.Info("configured actor sync completed", "source", sourceLabel, "count", len(actors))
	return nil
}
