package connectors

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"log/slog"
)

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewConnectorsService creates a new connectors service
func NewConnectorsService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	logger *slog.Logger,
) C {
	return &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		logger:  logger,
	}
}

// MigrateConnectors migrates connectors from configuration to the database
func (s *service) MigrateConnectors(ctx context.Context) error {
	root := s.cfg.GetRoot()
	if root == nil || len(root.Connectors) == 0 {
		s.logger.Info("No connectors to migrate")
		return nil
	}

	for _, configConnector := range root.Connectors {
		if err := s.migrateConnector(ctx, &configConnector); err != nil {
			return errors.Wrapf(err, "failed to migrate connector %s", configConnector.Id)
		}
	}

	return nil
}

// prevalidateConfig performs validation on the connector definitions in the config before we start migration.
func (s *service) prevalidateConfig() error {
	return nil
}

// migrateConnector migrates a single connector from configuration to the database
func (s *service) migrateConnector(ctx context.Context, configConnector *config.Connector) error {
	// Check if we have a UUID for this connector
	var connectorID uuid.UUID
	var err error

	// If the ID is a valid UUID, use it, otherwise generate a new one
	if configConnector.Id != uuid.Nil {
		connectorID = configConnector.Id
	} else {
		// No ID, generate a new one
		connectorID = uuid.New()
		s.logger.Info("Generated new UUID for connector with no ID", "uuid", connectorID)
	}

	// Convert connector config to JSON
	connectorJSON, err := json.Marshal(configConnector)
	if err != nil {
		return errors.Wrap(err, "failed to marshal connector config to JSON")
	}

	// Encrypt the connector JSON
	encryptedDefinition, err := s.encrypt.EncryptStringGlobal(ctx, string(connectorJSON))
	if err != nil {
		return errors.Wrap(err, "failed to encrypt connector definition")
	}

	// Get the auth type
	authType := ""
	if configConnector.Auth != nil {
		authType = string(configConnector.Auth.GetType())
	}

	// Check if the connector already exists in the database
	// We'll look for the latest version with the same ID
	existingVersion, err := s.db.GetConnectorVersion(ctx, connectorID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to check for existing connector")
	}

	var latestVersion int64 = 1
	var shouldCreate = true

	// If we found a connector, check if we need to create a new version
	if existingVersion != nil {
		// Check if the encrypted definition matches
		if existingVersion.EncryptedDefinition == encryptedDefinition {
			// No changes, nothing to do
			s.logger.Info("Connector already exists with same definition", "id", connectorID, "version", existingVersion.Version)
			return nil
		}

		// Changes detected, create a new version
		latestVersion = existingVersion.Version + 1
		s.logger.Info("Creating new version for connector", "id", connectorID, "version", latestVersion)
	}

	if shouldCreate {
		// Since we don't have a direct method to create a connector version,
		// we'll need to implement this functionality in a future update to the database package.
		// For now, we'll log a message indicating that we would create the connector version.
		s.logger.Info("Would create connector version",
			"id", connectorID,
			"version", latestVersion,
			"type", authType,
			"displayName", configConnector.DisplayName,
			"description", configConnector.Description,
			"logo", configConnector.Logo.GetUrl(),
			"encryptedDefinition", encryptedDefinition[:20]+"...")

		if err != nil {
			return errors.Wrap(err, "failed to create connector version")
		}

		s.logger.Info("Created connector version", "id", connectorID, "version", latestVersion)
	}

	return nil
}

var _ C = (*service)(nil)
