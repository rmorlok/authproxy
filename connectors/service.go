package connectors

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/util"
	"log/slog"
	"strings"
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
		if err := s.precheckConnectorForMigration(ctx, &configConnector); err != nil {
			return err
		}
	}

	for _, configConnector := range root.Connectors {
		if err := s.migrateConnector(ctx, &configConnector); err != nil {
			return err
		}
	}

	return nil
}

func (s *service) configConnectorToVersion(configConnector *config.Connector) (*database.ConnectorVersion, error) {
	connectorJSON, err := json.Marshal(configConnector)
	if err != nil {
		return nil, err
	}

	encryptedDefinition, err := s.encrypt.EncryptStringGlobal(context.Background(), string(connectorJSON))
	if err != nil {
		return nil, err
	}

	return &database.ConnectorVersion{
		ID:                  configConnector.Id,
		Version:             int64(configConnector.Version),
		Type:                configConnector.Type,
		Hash:                configConnector.Hash(),
		State:               database.ConnectorVersionStateDraft,
		EncryptedDefinition: encryptedDefinition,
	}, nil
}

// precheckConnectorForMigration checks the database to see if the connector definition aligns with the current state.
// This covers enforcement that a version that is published cannot change, and what identifiers are required to
// differentiate this connector definition from others that exist.
func (s *service) precheckConnectorForMigration(ctx context.Context, configConnector *config.Connector) error {
	if configConnector.HasId() {
		if configConnector.HasVersion() {
			existingVersion, err := s.db.GetConnectorVersion(ctx, configConnector.Id, configConnector.Version)
			if err != nil {
				return errors.Wrap(err, "failed to check for existing connector for precheck")
			}

			if existingVersion == nil {
				// Check for other versions that might exist
				newestVersion, err := s.db.NewestConnectorVersionForId(ctx, configConnector.Id)
				if err != nil {
					return errors.Wrap(err, "failed to get newest version of connector for precheck")
				}

				if newestVersion != nil && uint64(newestVersion.Version+1) != configConnector.Version {
					return errors.Errorf("connector %s currently has version %d and cannot be incremented to %d", configConnector.Id, newestVersion.Version, configConnector.Version)
				}

				if newestVersion == nil && configConnector.Version != 1 {
					return errors.Errorf("connector %s does does not have previous versions and must start with version 1", configConnector.Id)
				}
			} else {
				if existingVersion.State != database.ConnectorVersionStateDraft && existingVersion.Hash != configConnector.Hash() {
					return errors.Errorf("connector %s version %d has been published and cannot be modified", configConnector.Id, configConnector.Version)
				}
			}
		}
	} else {
		// No connector id means that connector type must be unique by id
		b := s.db.ListConnectorsBuilder().
			ForType(configConnector.Type).
			OrderBy(database.ConnectorOrderByCreatedAt, database.OrderByDesc).
			Limit(100)

		results := make([]database.Connector, 0)
		err := b.Enumerate(ctx, func(result database.PageResult[database.Connector]) (keepGoing bool, err error) {
			results = append(results, result.Results...)
			return true, nil
		})
		if err != nil {
			return errors.Wrap(err, "failed to check for existing connector for precheck")
		}

		if len(results) > 1 {
			connectorIds := strings.Join(util.Map(results, func(c database.Connector) string { return c.ID.String() }), ", ")
			return errors.Errorf("connector type %s is not unique among existing defined connectors: %s", configConnector.Type, connectorIds)
		}
	}

	return nil
}

// migrateConnector migrates a single connector from configuration to the database
func (s *service) migrateConnector(ctx context.Context, configConnector *config.Connector) error {
	b := newVersionBuilder(s)

	if configConnector.HasId() && configConnector.HasVersion() {
		cv, err := b.
			WithId(configConnector.Id).
			WithVersion(int64(configConnector.Version)).
			WithConfig(configConnector).
			Build()
		if err != nil {
			return errors.Wrap(err, "failed to build connector version")
		}

		err = s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion)
		if err != nil {
			return errors.Wrap(err, "failed to upsert connector version")
		}
	} else if configConnector.HasId() {
		existingVersion, err := s.db.NewestConnectorVersionForId(ctx, configConnector.Id)
		if err != nil {
			return errors.Wrap(err, "failed to get newest version of connector")
		}

		cv, err := b.
			WithId(configConnector.Id).
			WithVersion(existingVersion.Version + 1).
			WithConfig(configConnector).
			Build()
		if err != nil {
			return errors.Wrap(err, "failed to build connector version")
		}

		err = s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion)
		if err != nil {
			return errors.Wrap(err, "failed to upsert connector version")
		}
	} else if configConnector.HasVersion() {
		existingVersion, err := s.db.GetConnectorVersionForTypeAndVersion(ctx, configConnector.Type, configConnector.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get connector version for type/version")
		}

		cv, err := b.
			WithId(existingVersion.ID).
			WithVersion(int64(configConnector.Version)).
			WithConfig(configConnector).
			Build()
		if err != nil {
			return errors.Wrap(err, "failed to build connector version")
		}

		err = s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion)
		if err != nil {
			return errors.Wrap(err, "failed to upsert connector version")
		}
	} else {
		existingVersion, err := s.db.GetConnectorVersionForType(ctx, configConnector.Type)
		if err != nil {
			return errors.Wrap(err, "failed to get connector version for type")
		}

		cv, err := b.
			WithId(existingVersion.ID).
			WithVersion(existingVersion.Version + 1).
			WithConfig(configConnector).
			Build()
		if err != nil {
			return errors.Wrap(err, "failed to build connector version")
		}

		err = s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion)
		if err != nil {
			return errors.Wrap(err, "failed to upsert connector version")
		}
	}

	return nil
}

var _ C = (*service)(nil)
