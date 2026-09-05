package core

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "connectors-migrate-lock"

// connectorSourceLabelKey is an apxy/-prefixed system label written on every
// connector version that originates from the config-file migration mechanism.
// It lets the migration orphan-cleanup pass distinguish config-managed
// connectors from those created via the API.
const connectorSourceLabelKey = "apxy/cxr/source"

// connectorSourceLabelValueConfig is the value written under
// connectorSourceLabelKey for connector versions sourced from the config file.
const connectorSourceLabelValueConfig = "config"

const rateLimitSourceLabelKey = "apxy/rl/source"
const rateLimitSourceLabelValueConfig = "config"

// Migrate all resources from the config file into the system, triggering
// appropriate event hooks, etc.
func (s *service) Migrate(ctx context.Context) error {
	err := s.MigrateNamespaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to migrate namespaces: %w", err)
	}

	err = s.MigrateConnectors(ctx)
	if err != nil {
		return fmt.Errorf("failed to migrate connectors: %w", err)
	}

	err = s.MigrateRateLimits(ctx)
	if err != nil {
		return fmt.Errorf("failed to migrate rate limits: %w", err)
	}

	return nil
}

func (s *service) MigrateNamespaces(ctx context.Context) error {
	namespaces := []string{namespace.Root}

	cfgRoot := s.cfg.GetRoot()
	if cfgRoot == nil {
		return errors.New("invalid config")
	}

	for _, configConnector := range cfgRoot.Connectors.GetConnectors() {
		namespaces = append(namespaces, configConnector.GetNamespace())
	}
	for _, rateLimit := range cfgRoot.RateLimits.GetRateLimits() {
		namespaces = append(namespaces, rateLimit.Metadata.Namespace)
	}

	prefixOrderedList := namespace.SplitPathsToPrefixes(namespaces)

	// Because prefixOrderedList is in the appropriate order, this list will
	// also be in the appropriate order
	toCreatePaths := make([]string, 0)

	// Precheck to make sure there aren't going to be errors in migration
	for _, nsPath := range prefixOrderedList {
		ns, err := s.db.GetNamespace(ctx, nsPath)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				toCreatePaths = append(toCreatePaths, nsPath)
				continue
			} else {
				return fmt.Errorf("failed to get namespace: %w", err)
			}
		}

		if ns.State != database.NamespaceStateActive {
			return fmt.Errorf("namespace %s is not active", nsPath)
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
		err := s.db.CreateNamespace(ctx, &database.Namespace{
			Path:   nsPath,
			State:  database.NamespaceStateActive,
			Labels: make(database.Labels),
		})
		if err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", nsPath, err)
		}
	}

	s.logger.Info("finished migrating namespaces", "migrated_count", len(prefixOrderedList))

	return nil
}

// MigrateRateLimits reconciles canonical RateLimit resources from the config
// file after connectors, allowing typed connector references to resolve to the
// identities created in the same startup migration.
func (s *service) MigrateRateLimits(ctx context.Context) error {
	cfgRoot := s.cfg.GetRoot()
	if cfgRoot == nil {
		return errors.New("invalid config")
	}

	if cfgRoot.RateLimits == nil {
		s.logger.Info("no rate limits configured")
		return nil
	}

	if err := cfgRoot.RateLimits.Validate(
		&scommon.ValidationContext{Path: "$.rateLimits"},
	); err != nil {
		return fmt.Errorf("invalid rate-limit configuration: %w", err)
	}

	seen := make(map[apid.ID]struct{}, len(cfgRoot.RateLimits.GetRateLimits()))
	for i := range cfgRoot.RateLimits.LoadFromList {
		id, err := s.migrateRateLimit(ctx, &cfgRoot.RateLimits.LoadFromList[i])
		if err != nil {
			return err
		}
		seen[id] = struct{}{}
	}

	return s.cleanupOrphanedConfigRateLimits(ctx, seen)
}

func (s *service) migrateRateLimit(
	ctx context.Context,
	configured *rlschema.RateLimit,
) (apid.ID, error) {
	desired := configured.Clone()

	if err := s.normalizeRateLimitScope(ctx, desired); err != nil {
		return apid.Nil, fmt.Errorf("failed to resolve configured rate-limit scope: %w", err)
	}

	var existing *database.RateLimit
	var id apid.ID

	if desired.Metadata.ID != "" {
		parsed, err := apid.Parse(desired.Metadata.ID)
		if err != nil {
			return apid.Nil, err
		}

		id = parsed
		existing, err = s.db.GetRateLimit(ctx, id)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apid.Nil, err
		}
	} else {
		found, err := s.rateLimitForConfigName(
			ctx,
			desired.Metadata.Namespace,
			desired.Metadata.Name,
		)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apid.Nil, err
		}

		existing = found
		if existing != nil {
			id = existing.Id
		} else {
			id = apid.New(apid.PrefixRateLimit)
		}
	}

	if existing == nil {
		if desired.Metadata.Labels == nil {
			desired.Metadata.Labels = map[string]string{}
		}

		desired.Metadata.Labels[rateLimitSourceLabelKey] = rateLimitSourceLabelValueConfig
		desired = desired.ApplyCreateDefaults(id)

		row, err := databaseRateLimitFromResource(desired, id)
		if err != nil {
			return apid.Nil, err
		}

		if err := s.db.CreateRateLimit(ctx, row); err != nil {
			return apid.Nil, fmt.Errorf("failed to create configured rate limit %s: %w", id, err)
		}

		return id, nil
	}

	current := rateLimitResourceFromDatabase(*existing)

	if desired.Metadata.Name != "" {
		current.Metadata.Name = desired.Metadata.Name
	}

	current.Metadata.Labels = maps.Clone(desired.Metadata.Labels)
	current.Metadata.Annotations = maps.Clone(desired.Metadata.Annotations)
	current.Spec = desired.Spec.Clone()

	if _, err := s.UpdateRateLimit(ctx, id, current); err != nil {
		return apid.Nil, fmt.Errorf("failed to update configured rate limit %s: %w", id, err)
	}

	return id, nil
}

func (s *service) rateLimitForConfigName(
	ctx context.Context,
	namespace string,
	name scommon.ResourceName,
) (*database.RateLimit, error) {
	page := s.db.
		ListRateLimitsBuilder().
		ForNamespaceMatchers([]string{namespace}).
		ForName(name).
		Limit(2).
		FetchPage(ctx)

	if page.Error != nil {
		return nil, page.Error
	}

	if len(page.Results) == 0 {
		return nil, database.ErrNotFound
	}

	if len(page.Results) > 1 {
		return nil, fmt.Errorf("multiple rate limits named %q exist in namespace %q: %w", name, namespace, database.ErrViolation)
	}

	return &page.Results[0], nil
}

func (s *service) cleanupOrphanedConfigRateLimits(
	ctx context.Context,
	seen map[apid.ID]struct{},
) error {
	selector := fmt.Sprintf(
		"%s=%s",
		rateLimitSourceLabelKey,
		rateLimitSourceLabelValueConfig,
	)

	return s.db.
		ListRateLimitsBuilder().
		ForLabelSelector(selector).
		Enumerate(ctx, func(page pagination.PageResult[database.RateLimit]) (pagination.KeepGoing, error) {
			for i := range page.Results {
				if _, ok := seen[page.Results[i].Id]; ok {
					continue
				}

				if err := s.db.DeleteRateLimit(
					ctx,
					page.Results[i].Id,
				); err != nil && !errors.Is(err, database.ErrNotFound) {
					return pagination.Stop, err
				}
			}

			return pagination.Continue, nil
		})
}

func (s *service) syncConfiguredConnectorEnvelope(
	ctx context.Context,
	configured *config.Connector,
	existing *database.ConnectorWithDefinition,
) error {
	if existing == nil {
		return nil
	}

	storedUserLabels, _ := database.SplitUserAndApxyLabels(existing.Labels)
	if !maps.Equal(
		map[string]string(storedUserLabels),
		configured.Metadata.Labels,
	) {
		if _, err := s.db.UpdateConnectorLabels(
			ctx,
			existing.Id,
			configured.Metadata.Labels,
		); err != nil {
			return fmt.Errorf("failed to update configured connector labels: %w", err)
		}
		s.enqueueConnectorLabelPropagation(ctx, existing.Id)
	}

	if !maps.Equal(
		map[string]string(existing.Annotations),
		configured.Metadata.Annotations,
	) {
		if _, err := s.db.UpdateConnectorAnnotations(
			ctx,
			existing.Id,
			configured.Metadata.Annotations,
		); err != nil {
			return fmt.Errorf("failed to update configured connector annotations: %w", err)
		}
	}

	desiredState := database.ConnectorDefinitionVersionStatePrimary
	if configured.Spec.Release.DesiredState != "" {
		desiredState = database.ConnectorDefinitionVersionState(configured.Spec.Release.DesiredState)
	}
	if existing.State != desiredState {
		if err := s.db.SetConnectorDefinitionVersionState(
			ctx,
			existing.Id,
			existing.Version,
			desiredState,
		); err != nil {
			return fmt.Errorf("failed to update configured connector release state: %w", err)
		}
	}

	return nil
}

// MigrateConnectors migrates connectors from configuration to the database. It
// should generally not be called directly, but call the Migrate(...) method
// instead to migrate everything.
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

	seen := make(map[apid.ID]struct{}, len(cfgRoot.Connectors.GetConnectors()))
	for _, configConnector := range cfgRoot.Connectors.GetConnectors() {
		resolvedId, err := s.migrateConnector(ctx, &configConnector)
		if err != nil {
			return err
		}
		if resolvedId != apid.Nil {
			seen[resolvedId] = struct{}{}
		}
	}

	if err := s.cleanupOrphanedConfigConnectors(ctx, seen); err != nil {
		return fmt.Errorf("failed to cleanup orphaned config connectors: %w", err)
	}

	return nil
}

// cleanupOrphanedConfigConnectors finds connectors that were previously
// loaded from a config file (carry the apxy/cxr/source=config label) but are
// no longer present in the current config. For each orphan:
//   - if no live connections exist, the connector is soft-deleted;
//   - if live connections remain, the most recent published version is
//     transitioned from primary to active so no new connections can be
//     created against it, and a warning is logged instructing the operator to
//     remove the connections via the API before the connector can be removed.
func (s *service) cleanupOrphanedConfigConnectors(
	ctx context.Context,
	seen map[apid.ID]struct{},
) error {
	selector := fmt.Sprintf(
		"%s=%s",
		connectorSourceLabelKey,
		connectorSourceLabelValueConfig,
	)

	return s.db.ListConnectorsBuilder().
		ForLabelSelector(selector).
		Enumerate(ctx, func(
			page pagination.PageResult[database.ConnectorWithDefinition],
		) (pagination.KeepGoing, error) {
			for _, connector := range page.Results {
				if _, kept := seen[connector.Id]; kept {
					continue
				}

				if err := s.handleOrphanedConfigConnector(
					ctx,
					&connector,
				); err != nil {
					return pagination.Stop, err
				}
			}
			return pagination.Continue, nil
		})
}

func (s *service) handleOrphanedConfigConnector(
	ctx context.Context,
	connector *database.ConnectorWithDefinition,
) error {
	hasConnections, err := s.connectorHasLiveConnections(ctx, connector.Id)
	if err != nil {
		return fmt.Errorf("failed to check for connections on orphaned connector %s: %w", connector.Id, err)
	}

	if !hasConnections {
		s.logger.Info(
			"removing orphaned config-sourced connector with no remaining connections",
			"connector_id", connector.Id,
			"namespace", connector.Namespace,
		)
		if err := s.db.DeleteConnector(
			ctx,
			connector.Id,
		); err != nil && !errors.Is(err, database.ErrNotFound) {
			return fmt.Errorf("failed to delete orphaned connector %s: %w", connector.Id, err)
		}
		return nil
	}

	// Connections still reference this connector — demote the most recently
	// published version from primary to active so no new connections can be
	// created against it, then surface a warning instructing the operator
	// what to do next.
	newest, err := s.db.NewestPublishedConnectorDefinitionVersionForId(
		ctx,
		connector.Id,
	)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("failed to get newest published version of orphaned connector %s: %w", connector.Id, err)
	}

	if newest != nil &&
		newest.State == database.ConnectorDefinitionVersionStatePrimary {
		if err := s.db.SetConnectorDefinitionVersionState(ctx, newest.Id, newest.Version, database.ConnectorDefinitionVersionStateActive); err != nil {
			return fmt.Errorf("failed to demote orphaned connector %s version %d: %w", newest.Id, newest.Version, err)
		}
	}

	s.logger.Warn(
		"orphaned config-sourced connector retains live connections; demoted to active and awaiting API action to remove",
		"connector_id", connector.Id,
		"namespace", connector.Namespace,
	)

	return nil
}

func (s *service) connectorHasLiveConnections(
	ctx context.Context,
	id apid.ID,
) (bool, error) {
	page := s.db.ListConnectionsBuilder().
		ForConnectorId(id).
		Limit(1).
		FetchPage(ctx)
	if page.Error != nil {
		return false, page.Error
	}
	return len(page.Results) > 0, nil
}

func (s *service) connectorVersionHashEquals(
	cv *database.ConnectorWithDefinition,
	expected string,
) (bool, error) {
	if cv == nil {
		return false, nil
	}
	actual, err := wrapConnector(*cv, s).getHash()
	if err != nil {
		return false, fmt.Errorf("failed to derive connector version hash: %w", err)
	}
	return actual == expected, nil
}

func (s *service) precheckConnectorsForMigration(
	ctx context.Context,
	configConnectors *config.Connectors,
) error {
	if err := configConnectors.ValidateIdentities(
		&scommon.ValidationContext{},
	); err != nil {
		return fmt.Errorf("invalid connector configuration: %w", err)
	}

	for _, configConnector := range configConnectors.GetConnectors() {
		if err := s.precheckConnectorForMigration(
			ctx,
			&configConnector,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *service) connectorForConfigName(ctx context.Context, namespace string, name scommon.ResourceName) (*database.ConnectorWithDefinition, error) {
	page := s.db.ListConnectorsBuilder().
		ForName(name).
		ForNamespaceMatchers([]string{namespace}).
		Limit(2).
		FetchPage(ctx)
	if page.Error != nil {
		return nil, page.Error
	}
	if len(page.Results) == 0 {
		return nil, database.ErrNotFound
	}
	if len(page.Results) > 1 {
		return nil, fmt.Errorf("multiple connectors named %q exist in namespace %q: %w", name, namespace, database.ErrViolation)
	}
	return &page.Results[0], nil
}

// precheckConnectorForMigration checks the database to see if the connector
// definition aligns with the current state. This covers enforcement that a
// version that is published cannot change, and what identifiers are required to
// differentiate this connector definition from others that exist.
func (s *service) precheckConnectorForMigration(
	ctx context.Context,
	configConnector *config.Connector,
) error {
	// Don't modify original as we do all the checks
	configConnector = configConnector.Clone()

	if configConnector.HasId() {
		if configConnector.HasName() {
			byName, err := s.connectorForConfigName(
				ctx,
				configConnector.GetNamespace(),
				configConnector.Metadata.Name,
			)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("failed to check connector name during precheck: %w", err)
			}

			if byName != nil && byName.Id != configConnector.GetId() {
				return fmt.Errorf("connector name %q already belongs to connector %s in namespace %q", configConnector.Metadata.Name, byName.Id, configConnector.GetNamespace())
			}
		}

		if configConnector.HasVersion() {
			existingVersion, err := s.db.GetConnectorDefinitionVersion(
				ctx,
				configConnector.GetId(),
				configConnector.Metadata.Generation,
			)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("failed to check for existing connector for precheck: %w", err)
			}

			if errors.Is(err, database.ErrNotFound) {
				// Check for other versions that might exist
				newestVersion, err := s.db.NewestConnectorDefinitionVersionForId(
					ctx,
					configConnector.GetId(),
				)
				if err != nil && !errors.Is(err, database.ErrNotFound) {
					return fmt.Errorf("failed to get newest version of connector for precheck: %w", err)
				}

				if newestVersion != nil {
					if newestVersion.Version+1 != configConnector.Metadata.Generation {
						return fmt.Errorf("connector %s currently has version %d and cannot be incremented to %d", configConnector.GetId(), newestVersion.Version, configConnector.Metadata.Generation)
					}

					if newestVersion.Namespace != configConnector.GetNamespace() {
						return fmt.Errorf("connector %s currently has namespace path '%s' and cannot be changed to '%s'", configConnector.GetId(), newestVersion.Namespace, configConnector.GetNamespace())
					}
				}

				if newestVersion == nil &&
					configConnector.Metadata.Generation != 1 {
					return fmt.Errorf("connector %s does does not have previous versions and must start with version 1", configConnector.GetId())
				}
			} else {
				if configConnector.Spec.Release.DesiredState == "" {
					// Unless specified, this is trying to be the primary version; important for hash
					configConnector.Spec.Release.DesiredState = "primary"
				}

				if existingVersion.Namespace != configConnector.GetNamespace() {
					return fmt.Errorf("connector %s currently has namespace '%s' and cannot be changed to %s", configConnector.GetId(), existingVersion.Namespace, configConnector.GetNamespace())
				}

				if existingVersion.State != database.ConnectorDefinitionVersionStateDraft {
					matches, hashErr := s.connectorVersionHashEquals(
						existingVersion,
						configConnector.DefinitionHash(),
					)
					if hashErr != nil {
						return hashErr
					}

					if !matches {
						return fmt.Errorf("connector %s version %d has been published and cannot be modified", configConnector.GetId(), configConnector.Metadata.Generation)
					}
				}
			}
		} else {
			existingVersion, err := s.db.NewestConnectorDefinitionVersionForId(
				ctx,
				configConnector.GetId(),
			)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("failed to get newest version of connector for precheck: %w", err)
			}
			if existingVersion != nil &&
				existingVersion.Namespace != configConnector.GetNamespace() {
				return fmt.Errorf("connector %s currently has namespace path %q and cannot be changed to %q", configConnector.GetId(), existingVersion.Namespace, configConnector.GetNamespace())
			}
		}
	} else {
		existingConnector, err := s.connectorForConfigName(
			ctx,
			configConnector.GetNamespace(),
			configConnector.Metadata.Name,
		)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return fmt.Errorf("failed to check for existing connector by name during precheck: %w", err)
		}

		if configConnector.HasVersion() {
			var existingVersion *database.ConnectorWithDefinition
			if existingConnector != nil {
				existingVersion, err = s.db.GetConnectorDefinitionVersion(
					ctx,
					existingConnector.Id,
					configConnector.Metadata.Generation,
				)
				if err != nil && !errors.Is(err, database.ErrNotFound) {
					return fmt.Errorf("failed to check for existing connector version for precheck: %w", err)
				}
			}

			if existingVersion == nil {
				if existingConnector == nil {
					if configConnector.Metadata.Generation != 1 {
						return fmt.Errorf("connector %q in namespace %q does not have previous versions and must start with version 1", configConnector.Metadata.Name, configConnector.GetNamespace())
					}
				} else {
					newestVersion, err := s.db.NewestConnectorDefinitionVersionForId(
						ctx,
						existingConnector.Id,
					)
					if err != nil && !errors.Is(err, database.ErrNotFound) {
						return fmt.Errorf("failed to get newest version of connector for precheck: %w", err)
					}
					if newestVersion != nil &&
						newestVersion.Version+1 != configConnector.Metadata.Generation {
						return fmt.Errorf("connector %q in namespace %q currently has version %d and cannot be incremented to %d", configConnector.Metadata.Name, configConnector.GetNamespace(), newestVersion.Version, configConnector.Metadata.Generation)
					}
				}
			} else {
				if configConnector.Spec.Release.DesiredState == "" {
					// Unless specified, this is trying to be the primary version; important for hash
					configConnector.Spec.Release.DesiredState = "primary"
				}

				if existingVersion.State != database.ConnectorDefinitionVersionStateDraft {
					matches, hashErr := s.connectorVersionHashEquals(
						existingVersion,
						configConnector.DefinitionHash(),
					)
					if hashErr != nil {
						return hashErr
					}

					if !matches {
						return fmt.Errorf("connector %q in namespace %q version %d has been published and cannot be modified", configConnector.Metadata.Name, configConnector.GetNamespace(), configConnector.Metadata.Generation)
					}
				}
			}
		}
	}

	return nil
}

// migrateConnector migrates a single connector from configuration to the
// database. Returns the resolved connector id (the id this config entry maps
// to in the database, whether newly generated or matched against an existing
// row) so the caller can track which connectors are config-managed and detect
// orphans.
func (s *service) syncConfiguredConnectorName(
	ctx context.Context,
	configConnector *config.Connector,
	existingVersion *database.ConnectorWithDefinition,
) error {
	if existingVersion == nil ||
		!configConnector.HasId() ||
		!configConnector.HasName() ||
		existingVersion.Name == configConnector.Metadata.Name {
		return nil
	}
	if err := s.UpdateConnectorName(
		ctx, configConnector.GetId(),
		configConnector.Metadata.Name,
	); err != nil {
		return fmt.Errorf("failed to rename configured connector %s to %q: %w", configConnector.GetId(), configConnector.Metadata.Name, err)
	}

	existingVersion.Name = configConnector.Metadata.Name

	return nil
}

func (s *service) migrateConnector(
	ctx context.Context,
	configConnector *config.Connector,
) (apid.ID, error) {
	b := newConnectorBuilder(s)

	id := apctx.GetIdGenerator(ctx).New(apid.PrefixConnector)
	if configConnector.HasId() {
		id = configConnector.GetId()
	}

	version := uint64(1)
	if configConnector.HasVersion() {
		version = configConnector.Metadata.Generation
	}

	state := database.ConnectorDefinitionVersionStatePrimary
	if configConnector.Spec.Release.DesiredState != "" {
		state = database.ConnectorDefinitionVersionState(
			configConnector.Spec.Release.DesiredState,
		)
	}

	var existingVersion *database.ConnectorWithDefinition
	var err error
	matchesExisting := func(candidate *Connector) (bool, error) {
		matches, err := s.connectorVersionHashEquals(
			existingVersion,
			candidate.Hash,
		)
		if err != nil || !matches {
			return matches, err
		}

		if err := s.syncConfiguredConnectorEnvelope(
			ctx,
			configConnector,
			existingVersion,
		); err != nil {
			return false, err
		}

		return true, nil
	}

	if configConnector.HasId() && configConnector.HasVersion() {
		existingVersion, err = s.db.GetConnectorDefinitionVersion(
			ctx,
			configConnector.GetId(),
			configConnector.Metadata.Generation,
		)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apid.Nil, fmt.Errorf("failed to get connector version: %w", err)
		}

		existingConnector := existingVersion
		if existingConnector == nil && configConnector.HasName() {
			existingConnector, err = s.db.NewestConnectorDefinitionVersionForId(
				ctx,
				configConnector.GetId(),
			)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return apid.Nil, fmt.Errorf("failed to get connector for rename: %w", err)
			}
		}

		if err := s.syncConfiguredConnectorName(
			ctx,
			configConnector,
			existingConnector,
		); err != nil {
			return apid.Nil, err
		}

		if existingVersion != nil {
			c, err := b.
				WithId(id).
				WithVersion(version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil {
				matches, hashErr := matchesExisting(c)
				if hashErr != nil {
					return apid.Nil, hashErr
				}
				if matches {
					// No update required
					return id, nil
				}
			}
		}
	} else if configConnector.HasId() {
		existingVersion, err = s.db.NewestConnectorDefinitionVersionForId(
			ctx,
			configConnector.GetId(),
		)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apid.Nil, fmt.Errorf("failed to get newest version of connector: %w", err)
		}
		if err := s.syncConfiguredConnectorName(
			ctx,
			configConnector,
			existingVersion,
		); err != nil {
			return apid.Nil, err
		}

		if existingVersion != nil {
			c, err := b.
				WithId(id).
				WithVersion(existingVersion.Version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil {
				matches, hashErr := matchesExisting(c)
				if hashErr != nil {
					return apid.Nil, hashErr
				}
				if matches {
					// No update required
					return id, nil
				}
			}

			version = existingVersion.Version + 1
		}
	} else if configConnector.HasVersion() {
		// Pattern C: version and name, no ID - resolve the logical connector by
		// exact name within its namespace, then address the version by its ID.
		existingConnector, lookupErr := s.connectorForConfigName(
			ctx,
			configConnector.GetNamespace(),
			configConnector.Metadata.Name,
		)
		if lookupErr != nil && !errors.Is(lookupErr, database.ErrNotFound) {
			return apid.Nil, fmt.Errorf("failed to get connector by name: %w", lookupErr)
		}
		if existingConnector != nil {
			id = existingConnector.Id
			existingVersion, err = s.db.GetConnectorDefinitionVersion(
				ctx,
				id,
				configConnector.Metadata.Generation,
			)
			if err != nil && !errors.Is(err, database.ErrNotFound) {
				return apid.Nil, fmt.Errorf("failed to get connector version by name: %w", err)
			}
			if existingVersion == nil {
				existingVersion = existingConnector
			}

			c, err := b.
				WithId(id).
				WithVersion(version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil {
				matches, hashErr := matchesExisting(c)
				if hashErr != nil {
					return apid.Nil, hashErr
				}
				if matches {
					// No update required
					return id, nil
				}
			}
		}
	} else {
		// Pattern D: name only - resolve the logical connector by exact name
		// within its namespace and let definition changes auto-increment version.
		existingVersion, err = s.connectorForConfigName(
			ctx,
			configConnector.GetNamespace(),
			configConnector.Metadata.Name,
		)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return apid.Nil, fmt.Errorf("failed to get connector by name: %w", err)
		}

		if existingVersion != nil {
			id = existingVersion.Id

			c, err := b.
				WithId(id).
				WithVersion(existingVersion.Version).
				WithConfig(configConnector).
				WithState(state).
				Build()

			if err == nil {
				matches, hashErr := matchesExisting(c)
				if hashErr != nil {
					return apid.Nil, hashErr
				}
				if matches {
					// No update required
					return id, nil
				}
			}

			version = existingVersion.Version + 1
		}
	}

	c, err := b.
		WithId(id).
		WithVersion(version).
		WithConfig(configConnector).
		WithState(state).
		Build()
	if err != nil {
		return apid.Nil, fmt.Errorf("failed to build connector version: %w", err)
	}
	// Name is config reconciliation metadata for the logical connector, not
	// part of the encrypted definition assembled by connectorBuilder.
	c.ConnectorWithDefinition.Name = configConnector.Metadata.Name

	// Final check, though this should be duplicative
	if existingVersion != nil {
		matches, hashErr := matchesExisting(c)
		if hashErr != nil {
			return apid.Nil, hashErr
		}
		if matches {
			// No update required
			return id, nil
		}
	}

	// Tag the version with the source marker so the orphan-cleanup pass can
	// distinguish config-managed connectors from API-created ones. Copy the
	// labels map first because the builder shared a reference with the
	// caller-owned config struct.
	taggedLabels := make(
		database.Labels,
		len(c.ConnectorWithDefinition.Labels)+1,
	)
	for k, v := range c.ConnectorWithDefinition.Labels {
		taggedLabels[k] = v
	}
	taggedLabels[connectorSourceLabelKey] = connectorSourceLabelValueConfig
	c.ConnectorWithDefinition.Labels = taggedLabels

	err = s.db.UpsertConnectorDefinitionVersion(ctx, &c.ConnectorWithDefinition)
	if err != nil {
		return apid.Nil, fmt.Errorf("failed to upsert connector version: %w", err)
	}

	return id, nil
}
