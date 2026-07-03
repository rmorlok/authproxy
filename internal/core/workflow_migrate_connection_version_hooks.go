package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func (s *service) applyMigrationHookForVersion(ctx context.Context, candidate *connectionMigrationCandidate, version *ConnectorVersion, sourceVersion, targetVersion uint64) error {
	def := version.GetDefinition()
	if def == nil || def.Migrations == nil {
		return nil
	}

	var hook *cschema.MigrationHook
	if targetVersion > sourceVersion {
		hook = def.Migrations.Up
	} else {
		hook = def.Migrations.Down
	}
	if hook == nil || hook.Javascript == "" {
		return nil
	}

	jsLib, err := version.getJavascriptLibrary()
	if err != nil {
		return err
	}
	result, err := apjs.NewContext(jsLib, map[string]any{
		"cfg":         candidate.Config,
		"labels":      candidate.UserLabels,
		"annotations": candidate.Annotations,
	}).EvaluateObject(hook.Javascript)
	if err != nil {
		return fmt.Errorf("evaluate migration hook for version %d: %w", version.Version, err)
	}

	patch, err := decodeMigrationHookPatch(result)
	if err != nil {
		return fmt.Errorf("decode migration hook for version %d: %w", version.Version, err)
	}
	if err := applyMigrationHookPatch(candidate, version, sourceVersion, targetVersion, patch); err != nil {
		return fmt.Errorf("apply migration hook for version %d: %w", version.Version, err)
	}
	return nil
}

func decodeMigrationHookPatch(raw map[string]any) (migrationHookPatch, error) {
	if len(raw) == 0 {
		return migrationHookPatch{}, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return migrationHookPatch{}, err
	}
	var patch migrationHookPatch
	if err := json.Unmarshal(b, &patch); err != nil {
		return migrationHookPatch{}, err
	}
	return patch, nil
}

func applyMigrationHookPatch(candidate *connectionMigrationCandidate, version *ConnectorVersion, sourceVersion, targetVersion uint64, patch migrationHookPatch) error {
	for key, value := range patch.Config.Set {
		candidate.Config[key] = value
	}
	for _, key := range patch.Config.Unset {
		delete(candidate.Config, key)
	}

	for key, value := range patch.Labels.Set {
		if err := database.ValidateUserLabelKey(key); err != nil {
			return err
		}
		if err := database.ValidateLabelValue(value); err != nil {
			return err
		}
		candidate.UserLabels[key] = value
	}
	for _, key := range patch.Labels.Unset {
		if err := database.ValidateUserLabelKey(key); err != nil {
			return err
		}
		delete(candidate.UserLabels, key)
	}

	for key, value := range patch.Annotations.Set {
		if err := database.ValidateAnnotationKey(key); err != nil {
			return err
		}
		candidate.Annotations[key] = value
	}
	for _, key := range patch.Annotations.Unset {
		if err := database.ValidateAnnotationKey(key); err != nil {
			return err
		}
		delete(candidate.Annotations, key)
	}

	for i, n := range patch.Notifications {
		upsert, err := migrationNotificationUpsert(candidate, version, sourceVersion, targetVersion, i, n)
		if err != nil {
			return err
		}
		candidate.Notifications = append(candidate.Notifications, upsert)
		candidate.NotificationKeys = append(candidate.NotificationKeys, upsert.Key)
	}
	return nil
}

func migrationNotificationUpsert(
	candidate *connectionMigrationCandidate,
	version *ConnectorVersion,
	sourceVersion uint64,
	targetVersion uint64,
	index int,
	def migrationNotificationDef,
) (database.NotificationUpsert, error) {
	if def.Title == "" {
		return database.NotificationUpsert{}, errors.New("migration notification title is required")
	}
	if def.Message == "" {
		return database.NotificationUpsert{}, errors.New("migration notification message is required")
	}
	level := database.NotificationLevel(def.Level)
	if level == "" {
		level = database.NotificationLevelInfo
	}
	if !database.IsValidNotificationLevel(level) {
		return database.NotificationUpsert{}, fmt.Errorf("invalid migration notification level %q", def.Level)
	}

	keyPart := def.Key
	if keyPart == "" {
		keyPart = fmt.Sprintf("v%d:%d", version.Version, index)
	}
	key := fmt.Sprintf("%s:%s:%d:%d:%s", connectionMigrationNotificationSource, candidate.Connection.Id, sourceVersion, targetVersion, keyPart)
	var actionURL *string
	if def.ActionURL != "" {
		actionURL = &def.ActionURL
	}
	source := connectionMigrationNotificationSource
	return database.NotificationUpsert{
		Key:          key,
		Level:        level,
		ResourceType: "connection",
		ResourceId:   candidate.Connection.Id,
		Namespace:    candidate.Connection.Namespace,
		Title:        def.Title,
		Message:      def.Message,
		ActionUrl:    actionURL,
		ViewPermissions: aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"get",
			candidate.Connection.Id.String(),
		),
		ActionPermissions: aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"update",
			candidate.Connection.Id.String(),
		),
		Source:   &source,
		Metadata: def.Metadata,
	}, nil
}
