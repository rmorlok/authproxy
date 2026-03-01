package core

import (
	"context"
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"strings"
)

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "connectors-migrate-lock"

// Migrate all resources from the config file into the system, triggering appropriate event hooks, etc.
func (s *service) Migrate(ctx context.Context) error {
	err := s.MigrateNamespaces(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to migrate namespaces")
	}

	err = s.MigrateConnectors(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to migrate connectors")
	}

	return nil
}

func (s *service) MigrateNamespaces(ctx context.Context) error {
	namespaces := []string{aschema.RootNamespace}

	cfgRoot := s.cfg.GetRoot()
	if cfgRoot == nil {
		return errors.New("invalid config")
	}

	for _, configConnector := range cfgRoot.Connectors.GetConnectors() {
		namespaces = append(namespaces, configConnector.GetNamespace())
	}

	prefixOrderedList := aschema.SplitNamespacePathsToPrefixes(namespaces)

	// Because prefixOrderedList is in the appropriate order, this list will also be in the appropriate order
	toCreatePaths := make([]string, 0)

	// Precheck to make sure there aren't going to be errors in migration
	for _, nsPath := range prefixOrderedList {
		ns, err := s.db.GetNamespace(ctx, nsPath)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				toCreatePaths = append(toCreatePaths, nsPath)
				continue
			} else {
				return errors.Wrap(err, "failed to get namespace")
			}
		}

		if ns.State != database.NamespaceStateActive {
			return errors.Errorf("namespace %s is not active", nsPath)
		}
	}

	if len(toCreatePaths) == 0 {
		s.logger.Info("no namespaces to migrate")
		return nil
	}

	s.logger.Info(
		"precheck passed, migrating namespaces",
		"namespace_count", len(prefixOrderedList),
		"to_migrate", len(toCreatePaths),
	)

	for _, nsPath := range toCreatePaths {
		s.logger.Info("migrating namespace", "namespace", nsPath)
		err := s.db.CreateNamespace(context.Background(), &database.Namespace{
			Path:   nsPath,
			State:  database.NamespaceStateActive,
			Labels: make(database.Labels),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create namespace %s", nsPath)
		}
	}

	s.logger.Info("finished migrating namespaces", "migrated_count", len(prefixOrderedList))

	return nil
}

// MigrateConnectors migrates connectors from configuration to the database. It should generally not be called
// directly, but call the Migrate(...) method instead to migrate everything.
func (s *service) MigrateConnectors(ctx context.Context) error {
	cfgRoot := s.cfg.GetRoot()
	if cfgRoot == nil {
		return errors.New("invalid config")
	}
	if len(cfgRoot.Connectors.GetConnectors()) == 0 {
		s.logger.Info("no connectors to migrate")
		return nil
	}

	if err := s.precheckConnectorsForMigration(ctx, cfgRoot.Connectors); err != nil {
		return err
	}
	s.logger.Info("precheck passed, migrating connectors", "connector_count", len(cfgRoot.Connectors.GetConnectors()))

	for _, configConnector := range cfgRoot.Connectors.GetConnectors() {
		if err := s.migrateConnector(ctx, cfgRoot.Connectors, &configConnector); err != nil {
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
		Id:                  configConnector.Id,
		Version:             configConnector.Version,
		Namespace:           configConnector.GetNamespace(),
		Labels:              configConnector.Labels,
		Hash:                configConnector.Hash(),
		State:               database.ConnectorVersionStateDraft,
		EncryptedDefinition: encryptedDefinition,
	}, nil
}

func (s *service) precheckConnectorsForMigration(ctx context.Context, configConnectors *config.Connectors) error {
	type IdVersionStateTuple struct {
		Id      apid.ID
		Version uint64
		State   string
	}

	type IdVersionTuple struct {
		Id      apid.ID
		Version uint64
	}

	type IdStateTuple struct {
		Id    apid.ID
		State string
	}

	identifyingLabels := configConnectors.GetIdentifyingLabels()

	idVersionStateCounts := make(map[IdVersionStateTuple]int)
	idVersionCounts := make(map[IdVersionTuple]int)
	idStateCounts := make(map[IdStateTuple]int)
	idCounts := make(map[apid.ID]int)
	identifyingLabelCounts := make(map[string]int)

	// Helper to serialize identifying label values for map key
	serializeLabels := func(c *config.Connector) string {
		labelValues := c.GetIdentifyingLabelValues(identifyingLabels)
		data, _ := json.Marshal(labelValues)
		return string(data)
	}

	for _, configConnector := range configConnectors.GetConnectors() {
		if err := s.precheckConnectorForMigration(ctx, configConnectors, &configConnector); err != nil {
			return err
		}

		labelKey := serializeLabels(&configConnector)

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

				ccLabelKey := serializeLabels(&cc)
				if ccLabelKey == labelKey && !cc.HasId() {
					return errors.Errorf("a connector with identifying labels %s exists with an id, but not all connectors with those labels have id", labelKey)
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

				ccLabelKey := serializeLabels(&cc)
				if ccLabelKey == labelKey && !cc.HasId() {
					return errors.Errorf("a connector with identifying labels %s exists with an id, but not all connectors with those labels have id", labelKey)
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

				ccLabelKey := serializeLabels(&cc)
				if ccLabelKey == labelKey && !cc.HasId() {
					return errors.Errorf("a connector with identifying labels %s exists with an id, but not all connectors with those labels have id", labelKey)
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

				ccLabelKey := serializeLabels(&cc)
				if ccLabelKey == labelKey && !cc.HasId() {
					return errors.Errorf("a connector with identifying labels %s exists with an id, but not all connectors with those labels have id", labelKey)
				}
			}
		} else {
			// Only identifying labels specified
			identifyingLabelCounts[labelKey]++
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

	for labelKey, count := range identifyingLabelCounts {
		if count > 1 {
			result = multierror.Append(result, errors.Errorf("connector with identifying labels %s has multiple entries without differentiating id or version", labelKey))
		}
	}

	return result.ErrorOrNil()
}

// precheckConnectorForMigration checks the database to see if the connector definition aligns with the current state.
// This covers enforcement that a version that is published cannot change, and what identifiers are required to
// differentiate this connector definition from others that exist.
func (s *service) precheckConnectorForMigration(ctx context.Context, configConnectors *config.Connectors, configConnector *config.Connector) error {
	// Don't modify original as we do all the checks
	configConnector = configConnector.Clone()
	identifyingLabels := configConnectors.GetIdentifyingLabels()

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
		// No connector id means that we need to look up by identifying labels
		labelValues := configConnector.GetIdentifyingLabelValues(identifyingLabels)
		labelSelector := database.BuildLabelSelectorFromMap(labelValues)

		b := s.db.ListConnectorsBuilder().
			ForLabelSelector(labelSelector).
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
			connectorIds := strings.Join(util.Map(results, func(c database.Connector) string { return c.Id.String() }), ", ")
			return errors.Errorf("connector with identifying labels %s is not unique among existing defined connectors: %s", labelSelector, connectorIds)
		} else if len(results) == 1 {
			if results[0].Namespace != configConnector.GetNamespace() {
				return errors.Errorf("connector %s currently has namespace path '%s' and cannot be changed to '%s'", configConnector.Id, results[0].Namespace, configConnector.GetNamespace())
			}
		}
	}

	return nil
}

// migrateConnector migrates a single connector from configuration to the database
func (s *service) migrateConnector(ctx context.Context, configConnectors *config.Connectors, configConnector *config.Connector) error {
	b := newConnectorVersionBuilder(s)
	identifyingLabels := configConnectors.GetIdentifyingLabels()

	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnectorVersion)
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
		// Pattern C: version only, no ID - use label-based lookup
		labelValues := configConnector.GetIdentifyingLabelValues(identifyingLabels)
		labelSelector := database.BuildLabelSelectorFromMap(labelValues)
		existingVersion, err := s.db.GetConnectorVersionForLabelsAndVersion(ctx, labelSelector, configConnector.Version)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return errors.Wrap(err, "failed to get connector version for labels/version")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(existingVersion.Id).
				WithVersion(version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}

			id = existingVersion.Id
		}
	} else {
		// Pattern D: no ID, no version - use label-based lookup
		labelValues := configConnector.GetIdentifyingLabelValues(identifyingLabels)
		labelSelector := database.BuildLabelSelectorFromMap(labelValues)
		existingVersion, err := s.db.GetConnectorVersionForLabels(ctx, labelSelector)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return errors.Wrap(err, "failed to get connector version for labels")
		}

		if existingVersion != nil {
			cv, err := b.
				WithId(existingVersion.Id).
				WithVersion(existingVersion.Version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil && cv.Hash == existingVersion.Hash {
				// No update required
				return nil
			}

			id = existingVersion.Id
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
