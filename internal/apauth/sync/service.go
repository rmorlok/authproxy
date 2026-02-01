package sync

import (
	"context"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
)

// Label constants for tracking configured actor sync source
const (
	// LabelConfiguredActorSyncSource is the label key used to identify the source of configured actors
	LabelConfiguredActorSyncSource = "authproxy.io/actor-sync-source"

	// LabelValueConfigList indicates the actor was synced from ConfiguredActorsList config
	LabelValueConfigList = "config-list"

	// LabelValuePublicKeyDir indicates the actor was synced from a directory containing
	// public keys for the actors, named by the actor external id
	LabelValuePublicKeyDir = "public-key-dir"
)

// Service handles synchronization of configured actors from configuration to the database.
type Service interface {
	// SyncActorList synchronizes actors from ConfiguredActorsList configuration to the database.
	// This is typically called at startup when ConfiguredActorsList is configured.
	SyncActorList(ctx context.Context) error

	// SyncConfiguredActorsExternalSource synchronizes actors from ConfiguredActorsExternalSource configuration to the database.
	// This is typically called periodically via cron when ConfiguredActorsExternalSource is configured.
	SyncConfiguredActorsExternalSource(ctx context.Context) error
}

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewService creates a new actor sync service.
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
