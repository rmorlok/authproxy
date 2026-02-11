package tasks

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
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

		if err := m.Lock(ctx); err != nil {
			if apredis.MutexIsErrNotObtained(err) {
				s.logger.Info("another sync is in progress, skipping this run")
				return nil
			}
			return errors.Wrap(err, "failed to acquire sync lock")
		}
		defer func() {
			if err := m.Unlock(ctx); err != nil {
				s.logger.Warn("failed to release sync lock", "error", err)
			}
		}()
	}

	return s.syncConfiguredActors(ctx, actors.All(), LabelValuePublicKeyDir)
}

// syncConfiguredActors performs the actual sync of configured actors to the database.
func (s *service) syncConfiguredActors(ctx context.Context, actors []*sconfig.ConfiguredActor, sourceLabel string) error {
	// Build a set of expected external IDs
	expectedExternalIds := make(map[string]bool)

	// Upsert each configured actor
	for _, actor := range actors {
		externalId := actor.ExternalId
		expectedExternalIds[externalId] = true

		// Serialize and encrypt the key
		var encryptedKey *string
		if actor.Key != nil {
			keyJson, err := json.Marshal(actor.Key)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal key for actor %s", actor.ExternalId)
			}

			encrypted, err := s.encrypt.EncryptStringGlobal(ctx, string(keyJson))
			if err != nil {
				return errors.Wrapf(err, "failed to encrypt key for actor %s", actor.ExternalId)
			}
			encryptedKey = &encrypted
		}

		// Create labels, starting with actor's configured labels
		labels := make(database.Labels)
		for k, v := range actor.Labels {
			labels[k] = v
		}
		// Add the sync source label
		labels[LabelConfiguredActorSyncSource] = sourceLabel

		// Create actor data with labels and encrypted key
		actorData := &configuredActorData{
			namespace:    "root",
			externalId:   externalId,
			labels:       labels,
			permissions:  actor.Permissions,
			encryptedKey: encryptedKey,
		}

		// Upsert the actor
		_, err := s.db.UpsertActor(ctx, actorData)
		if err != nil {
			return errors.Wrapf(err, "failed to upsert actor %s", actor.ExternalId)
		}

		s.logger.Debug("synced configured actor", "external_id", externalId)
	}

	// Delete stale actors (those with the sync label but not in current config)
	err := s.db.ListActorsBuilder().
		ForLabelSelector(LabelConfiguredActorSyncSource).
		Enumerate(ctx, func(result pagination.PageResult[*database.Actor]) (keepGoing bool, err error) {
			for _, dbActor := range result.Results {
				// Only delete actors with matching source label that aren't in current config
				if dbActor.Labels[LabelConfiguredActorSyncSource] == sourceLabel && !expectedExternalIds[dbActor.ExternalId] {
					s.logger.Info("deleting stale configured actor", "external_id", dbActor.ExternalId)
					if err := s.db.DeleteActor(ctx, dbActor.Id); err != nil {
						return false, errors.Wrapf(err, "failed to delete stale actor %s", dbActor.ExternalId)
					}
				}
			}
			return true, nil
		})

	if err != nil {
		return errors.Wrap(err, "failed to enumerate and cleanup stale actors")
	}

	s.logger.Info("configured actor sync completed", "source", sourceLabel, "count", len(actors))
	return nil
}
