package core

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"strings"
)

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "connectors-migrate-lock"

// Migrate all resources from the config file into the system, triggering appropriate event hooks, etc.
func (s *service) Migrate(ctx context.Context) error {
	err := s.MigrateConnectors(ctx)
	if err != nil {
		return err
	}

	return nil
}

// MigrateConnectors migrates connectors from configuration to the database. It should generally not be called
// directly, but call the Migrate(...) method instead to migrate everything.
func (s *service) MigrateConnectors(ctx context.Context) error {
	root := s.cfg.GetRoot()
	if root == nil || len(root.Connectors.GetConnectors()) == 0 {
		s.logger.Info("No connectors to migrate")
		return nil
	}

	if err := s.precheckConnectorsForMigration(ctx, root.Connectors); err != nil {
		return err
	}
	s.logger.Info("Precheck passed, migrating connectors", "connector_count", len(root.Connectors.GetConnectors()))

	for _, configConnector := range root.Connectors.GetConnectors() {
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
		Version:             configConnector.Version,
		Type:                configConnector.Type,
		Hash:                configConnector.Hash(),
		State:               database.ConnectorVersionStateDraft,
		EncryptedDefinition: encryptedDefinition,
	}, nil
}

func (s *service) precheckConnectorsForMigration(ctx context.Context, configConnectors *config.Connectors) error {
	type IdVersionStateTuple struct {
		Id      uuid.UUID
		Version uint64
		State   string
	}

	type IdVersionTuple struct {
		Id      uuid.UUID
		Version uint64
	}

	type IdStateTuple struct {
		Id    uuid.UUID
		State string
	}

	idVersionStateCounts := make(map[IdVersionStateTuple]int)
	idVersionCounts := make(map[IdVersionTuple]int)
	idStateCounts := make(map[IdStateTuple]int)
	idCounts := make(map[uuid.UUID]int)
	typeCounts := make(map[string]int)

	for _, configConnector := range configConnectors.GetConnectors() {
		if err := s.precheckConnectorForMigration(ctx, &configConnector); err != nil {
			return err
		}

		if configConnector.HasId() && configConnector.HasVersion() && configConnector.HasState() {
			idVersionStateCounts[IdVersionStateTuple{
				Id:      configConnector.Id,
				Version: configConnector.Version,
				State:   configConnector.State,
			}]++

			// All other entries for this id must have version and state specified if one does
			for _, cc := range configConnectors.GetConnectors() {
				if cc.Id == configConnector.Id && cc.Version == configConnector.Version && cc.State == configConnector.State {
					// Same entry we are checking
					continue
				}

				if cc.Id == configConnector.Id {
					if !cc.HasVersion() || (!configConnector.IsDraft() && !cc.HasState()) {
						return errors.Errorf("connector %s version %d has state %s but other entries do not have all these fields specified", configConnector.Id, configConnector.Version, configConnector.State)
					}
				}

				if cc.Type == configConnector.Type && !cc.HasId() {
					return errors.Errorf("a connector of type %s exists with an id, but not all connectors of that type have id", configConnector.Type)
				}
			}
		} else if configConnector.HasId() && configConnector.HasVersion() {
			idVersionCounts[IdVersionTuple{
				Id:      configConnector.Id,
				Version: configConnector.Version,
			}]++

			// All other entries for this id must have version if one does
			for _, cc := range configConnectors.GetConnectors() {
				if cc.Id == configConnector.Id && cc.Version == configConnector.Version {
					// Same entry we are checking
					continue
				}

				if cc.Id == configConnector.Id && !cc.HasVersion() {
					if !cc.HasVersion() {
						return errors.Errorf("connector %s has version %d has but not all other entries do not have version specified", configConnector.Id, configConnector.Version)
					}
				}

				if cc.Type == configConnector.Type && !cc.HasId() {
					return errors.Errorf("a connector of type %s exists with an id, but not all connectors of that type have id", configConnector.Type)
				}
			}
		} else if configConnector.HasId() && configConnector.HasState() {
			idStateCounts[IdStateTuple{
				Id:    configConnector.Id,
				State: configConnector.State,
			}]++

			// All other entries for this id must have version if one does
			for _, cc := range configConnectors.GetConnectors() {
				if cc.Id == configConnector.Id && cc.State == configConnector.State {
					// Same entry we are checking
					continue
				}

				if cc.Type == configConnector.Type && !cc.HasId() {
					return errors.Errorf("a connector of type %s exists with an id, but not all connectors of that type have id", configConnector.Type)
				}
			}
		} else if configConnector.HasId() {
			idCounts[configConnector.Id]++

			// All other entries for this id must have version if one does
			for _, cc := range configConnectors.GetConnectors() {
				if cc.Id == configConnector.Id && cc.State == configConnector.State {
					// Same entry we are checking
					continue
				}

				if cc.Type == configConnector.Type && !cc.HasId() {
					return errors.Errorf("a connector of type %s exists with an id, but not all connectors of that type have id", configConnector.Type)
				}
			}
		} else {
			// Only type specified
			typeCounts[configConnector.Type]++
		}
	}

	result := &multierror.Error{}

	for idVersionState, count := range idVersionStateCounts {
		if count > 1 {
			result = multierror.Append(result, errors.Errorf("connector %s version %d has multiple entries with state %s", idVersionState.Id, idVersionState.Version, idVersionState.State))
		}
	}

	for idVersion, count := range idVersionCounts {
		if count > 1 {
			result = multierror.Append(result, errors.Errorf("connector %s version %d has multiple entries without differentiating state", idVersion.Id, idVersion.Version))
		}
	}

	for id, count := range idCounts {
		if count > 1 {
			result = multierror.Append(result, errors.Errorf("connector %s has multiple entries without differentiating state or version", id))
		}
	}

	for typ, count := range typeCounts {
		if count > 1 {
			result = multierror.Append(result, errors.Errorf("connector type %s has multiple entries without differentiating id or version", typ))
		}
	}

	return result.ErrorOrNil()
}

// precheckConnectorForMigration checks the database to see if the connector definition aligns with the current state.
// This covers enforcement that a version that is published cannot change, and what identifiers are required to
// differentiate this connector definition from others that exist.
func (s *service) precheckConnectorForMigration(ctx context.Context, configConnector *config.Connector) error {
	// Don't modify original as we do all the checks
	configConnector = configConnector.Clone()

	if configConnector.HasId() {
		if configConnector.HasVersion() {
			existingVersion, err := s.db.GetConnectorVersion(ctx, configConnector.Id, configConnector.Version)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return errors.Wrap(err, "failed to check for existing connector for precheck")
			}

			if errors.Is(err, database.ErrNotFound) {
				// Check for other versions that might exist
				newestVersion, err := s.db.NewestConnectorVersionForId(ctx, configConnector.Id)
				if err != nil && !errors.Is(err, database.ErrNotFound) {
					return errors.Wrap(err, "failed to get newest version of connector for precheck")
				}

				if newestVersion != nil {
					if newestVersion.Version+1 != configConnector.Version {
						return errors.Errorf("connector %s currently has version %d and cannot be incremented to %d", configConnector.Id, newestVersion.Version, configConnector.Version)
					}

					if newestVersion.Namespace != configConnector.GetNamespace() {
						return errors.Errorf("connector %s currently has namespace path '%s' and cannot be changed to '%s'", configConnector.Id, newestVersion.Namespace, configConnector.GetNamespace())
					}
				}

				if newestVersion == nil && configConnector.Version != 1 {
					return errors.Errorf("connector %s does does not have previous versions and must start with version 1", configConnector.Id)
				}
			} else {
				if configConnector.State == "" {
					// Unless specified, this is trying to be the primary version; important for hash
					configConnector.State = string(database.ConnectorVersionStatePrimary)
				}

				if existingVersion.Namespace != configConnector.GetNamespace() {
					return errors.Errorf("connector %s currently has namespace '%s' and cannot be changed to %s", configConnector.Id, existingVersion.Namespace, configConnector.GetNamespace())
				}

				if existingVersion.State != database.ConnectorVersionStateDraft && existingVersion.Hash != configConnector.Hash() {
					return errors.Errorf("connector %s version %d has been published and cannot be modified", configConnector.Id, configConnector.Version)
				}
			}
		}
	} else {
		// No connector id means that connector type must be unique by id
		b := s.db.ListConnectorsBuilder().
			ForType(configConnector.Type).
			OrderBy(database.ConnectorOrderByCreatedAt, pagination.OrderByDesc).
			Limit(100)

		results := make([]database.Connector, 0)
		err := b.Enumerate(ctx, func(result pagination.PageResult[database.Connector]) (keepGoing bool, err error) {
			results = append(results, result.Results...)
			return true, nil
		})
		if err != nil {
			return errors.Wrap(err, "failed to check for existing connector for precheck")
		}

		if len(results) > 1 {
			connectorIds := strings.Join(util.Map(results, func(c database.Connector) string { return c.ID.String() }), ", ")
			return errors.Errorf("connector type %s is not unique among existing defined connectors: %s", configConnector.Type, connectorIds)
		} else if len(results) == 1 {
			if results[0].Namespace != configConnector.GetNamespace() {
				return errors.Errorf("connector %s currently has namespace path '%s' and cannot be changed to '%s'", configConnector.Id, results[0].Namespace, configConnector.GetNamespace())
			}
		}
	}

	return nil
}

// migrateConnector migrates a single connector from configuration to the database
func (s *service) migrateConnector(ctx context.Context, configConnector *config.Connector) error {
	b := newConnectorVersionBuilder(s)

	id := apctx.GetUuidGenerator(ctx).New()
	if configConnector.HasId() {
		id = configConnector.Id
	}

	version := uint64(1)
	if configConnector.HasVersion() {
		version = configConnector.Version
	}

	state := database.ConnectorVersionStatePrimary
	if configConnector.State != "" {
		state = database.ConnectorVersionState(configConnector.State)
	}

	var existingVersion *database.ConnectorVersion
	var err error

	if configConnector.HasId() && configConnector.HasVersion() {
		existingVersion, err = s.db.GetConnectorVersion(ctx, configConnector.Id, configConnector.Version)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return errors.Wrap(err, "failed to get connector version")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(id).
				WithVersion(version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}
		}
	} else if configConnector.HasId() {
		existingVersion, err = s.db.NewestConnectorVersionForId(ctx, configConnector.Id)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return errors.Wrap(err, "failed to get newest version of connector")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(id).
				WithVersion(existingVersion.Version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}

			version = existingVersion.Version + 1
		}
	} else if configConnector.HasVersion() {
		existingVersion, err := s.db.GetConnectorVersionForTypeAndVersion(ctx, configConnector.Type, configConnector.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get connector version for type/version")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(existingVersion.ID).
				WithVersion(version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}

			id = existingVersion.ID
		}
	} else {
		existingVersion, err := s.db.GetConnectorVersionForType(ctx, configConnector.Type)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return errors.Wrap(err, "failed to get connector version for type")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(existingVersion.ID).
				WithVersion(existingVersion.Version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}

			id = existingVersion.ID
			version = existingVersion.Version + 1
		}
	}

	cv, err := b.
		WithId(id).
		WithVersion(version).
		WithConfig(configConnector).
		WithState(state).
		Build()
	if err != nil {
		return errors.Wrap(err, "failed to build connector version")
	}

	// Final check, though this should be duplicative
	if existingVersion != nil && existingVersion.Hash == cv.Hash {
		// No update required
		return nil
	}

	err = s.db.UpsertConnectorVersion(ctx, &cv.ConnectorVersion)
	if err != nil {
		return errors.Wrap(err, "failed to upsert connector version")
	}

	return nil
}
