package tasks

import (
	"context"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
)

// Label constants for tracking admin sync source
const (
	// LabelAdminSyncSource is the label key used to identify the source of admin actors
	LabelAdminSyncSource = "authproxy.io/actor-sync-source"

	// LabelValueConfigList indicates the admin was synced from AdminUsersList config
	LabelValueConfigList = "config-list"

	// LabelValuePublicKeyDir indicates the admin was synced from a directory containing
	// public keys for the actors, named by the actor external id
	LabelValuePublicKeyDir = "public-key-dir"
)

// Service handles synchronization of admin users from configuration to the database.
type Service interface {
	// SyncActorList synchronizes admin users from AdminUsersList configuration to the database.
	// This is typically called at startup when AdminUsersList is configured.
	SyncActorList(ctx context.Context) error

	// SyncAdminUsersExternalSource synchronizes admin users from AdminUsersExternalSource configuration to the database.
	// This is typically called periodically via cron when AdminUsersExternalSource is configured.
	SyncAdminUsersExternalSource(ctx context.Context) error
}

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewService creates a new admin sync service.
func NewService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	logger *slog.Logger,
) Service {
	return &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		logger:  logger,
	}
}
