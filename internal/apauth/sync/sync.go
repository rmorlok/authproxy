package sync

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// SyncActorList synchronizes actors from AdminUsersList configuration to the database.
func (s *service) SyncActorList(ctx context.Context) error {
	adminUsers := s.cfg.GetRoot().SystemAuth.AdminUsers
	if adminUsers == nil {
		s.logger.Info("no admin users configured, skipping sync")
		return nil
	}

	// Check if this is an AdminUsersList (not external source)
	if _, ok := adminUsers.InnerVal.(sconfig.AdminUsersList); !ok {
		s.logger.Debug("admin users is not a list type, skipping list sync")
		return nil
	}

	return s.syncAdminUsers(ctx, adminUsers.All(), LabelValueConfigList)
}

// SyncAdminUsersExternalSource synchronizes admin users from AdminUsersExternalSource configuration to the database.
func (s *service) SyncAdminUsersExternalSource(ctx context.Context) error {
	adminUsers := s.cfg.GetRoot().SystemAuth.AdminUsers
	if adminUsers == nil {
		s.logger.Info("no admin users configured, skipping sync")
		return nil
	}

	// Check if this is an AdminUsersExternalSource
	if _, ok := adminUsers.InnerVal.(*sconfig.AdminUsersExternalSource); !ok {
		s.logger.Debug("admin users is not an external source type, skipping external source sync")
		return nil
	}

	return s.syncAdminUsers(ctx, adminUsers.All(), LabelValuePublicKeyDir)
}

// syncAdminUsers performs the actual sync of admin users to the database.
func (s *service) syncAdminUsers(ctx context.Context, adminUsers []*sconfig.AdminUser, sourceLabel string) error {
	// Build a set of expected external IDs
	expectedExternalIds := make(map[string]bool)

	// Get the admin email domain from config
	adminDomain := "local"
	if s.cfg.GetRoot().SystemAuth.AdminEmailDomain != "" {
		adminDomain = s.cfg.GetRoot().SystemAuth.AdminEmailDomain
	}

	// Upsert each admin user
	for _, adminUser := range adminUsers {
		externalId := fmt.Sprintf("admin/%s", adminUser.Username)
		expectedExternalIds[externalId] = true

		// Determine email
		email := fmt.Sprintf("%s@%s", adminUser.Username, adminDomain)
		if adminUser.Email != "" {
			email = adminUser.Email
		}

		// Serialize and encrypt the key
		var encryptedKey *string
		if adminUser.Key != nil {
			keyJson, err := json.Marshal(adminUser.Key)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal key for admin user %s", adminUser.Username)
			}

			encrypted, err := s.encrypt.EncryptStringGlobal(ctx, string(keyJson))
			if err != nil {
				return errors.Wrapf(err, "failed to encrypt key for admin user %s", adminUser.Username)
			}
			encryptedKey = &encrypted
		}

		// Create actor data with labels and encrypted key
		actorData := &adminActorData{
			namespace:  "root",
			externalId: externalId,
			email:      email,
			admin:      true,
			superAdmin: false,
			labels: database.Labels{
				LabelAdminSyncSource: sourceLabel,
			},
			permissions:  adminUser.Permissions,
			encryptedKey: encryptedKey,
		}

		// Upsert the actor
		_, err := s.db.UpsertActor(ctx, actorData)
		if err != nil {
			return errors.Wrapf(err, "failed to upsert admin actor %s", adminUser.Username)
		}

		s.logger.Debug("synced admin user", "username", adminUser.Username, "external_id", externalId)
	}

	// Delete stale admin actors (those with the sync label but not in current config)
	err := s.db.ListActorsBuilder().
		ForIsAdmin(true).
		ForLabelEquals(LabelAdminSyncSource, sourceLabel).
		Enumerate(ctx, func(result pagination.PageResult[*database.Actor]) (keepGoing bool, err error) {
			for _, actor := range result.Results {
				if !expectedExternalIds[actor.ExternalId] {
					s.logger.Info("deleting stale admin actor", "external_id", actor.ExternalId)
					if err := s.db.DeleteActor(ctx, actor.Id); err != nil {
						return false, errors.Wrapf(err, "failed to delete stale admin actor %s", actor.ExternalId)
					}
				}
			}
			return true, nil
		})

	if err != nil {
		return errors.Wrap(err, "failed to enumerate and cleanup stale admin actors")
	}

	s.logger.Info("admin user sync completed", "source", sourceLabel, "count", len(adminUsers))
	return nil
}
