package core

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const rateLimitSourceLabelKey = "apxy/rl/source"
const rateLimitSourceLabelValueConfig = "config"

// Migrate all resources from the config file into the system, triggering
// appropriate event hooks, etc.
func (s *service) Migrate(ctx context.Context) error {
	err := s.MigrateNamespaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to migrate namespaces: %w", err)
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

	for _, rateLimit := range cfgRoot.RateLimits.GetRateLimits() {
		namespaces = append(namespaces, rateLimit.Metadata.Namespace)
	}
	for _, configuredActor := range cfgRoot.SystemAuth.Actors.All() {
		namespaces = append(namespaces, configuredActor.GetNamespace())
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
// file. Connector references must already exist or be applied separately.
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
