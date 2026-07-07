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

// applyMigrationHookForVersion applies the javascript upgrade/downgrade hook,
// if one exists, for a connection version migration. This may modify the
// candidate by changing the config, user labels, and annotations, and/or
// adding/removing notifications.
func (s *service) applyMigrationHookForVersion(
	ctx context.Context,
	candidate *connectionMigrationCandidate,
	version *ConnectorVersion,
	sourceVersion,
	targetVersion uint64,
) error {
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
	if err := applyMigrationHookPatch(candidate, patch); err != nil {
		return fmt.Errorf("apply migration hook for version %d: %w", version.Version, err)
	}
	return nil
}

// decodeMigrationHookPatch decodes a migration hook patch from a
// map[string]any to the strongly typed migrationHookPatch. This
// is accomplished by unmarshalling the raw map into a
// migrationHookPatch.
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

// applyMigrationHookPatch applies the migration hook patch to the connection
// migration candidate. It applies validation logic on th values to be updated
// on the candidate and errors on invalid values.
func applyMigrationHookPatch(
	candidate *connectionMigrationCandidate,
	patch migrationHookPatch,
) error {
	// Update the config settings
	for key, value := range patch.Config.Set {
		candidate.Config[key] = value
	}
	for _, key := range patch.Config.Unset {
		delete(candidate.Config, key)
	}

	// Update the user labels
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

	// Update the annotations
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

	// Update the notifications
	for _, n := range patch.Notifications.Set {
		upsert, rank, err := migrationNotificationUpsert(candidate, n)
		if err != nil {
			return err
		}
		setCandidateNotification(candidate, rank, upsert)
	}
	for _, n := range patch.Notifications.Unset {
		if err := unsetMigrationNotification(candidate, n); err != nil {
			return err
		}
	}
	return nil
}

func migrationNotificationUpsert(
	candidate *connectionMigrationCandidate,
	def migrationNotificationDef,
) (database.NotificationUpsert, int, error) {
	if def.Title == "" {
		return database.NotificationUpsert{}, 0, errors.New("migration notification title is required")
	}
	if def.Message == "" {
		return database.NotificationUpsert{}, 0, errors.New("migration notification message is required")
	}
	level := database.NotificationLevel(def.Level)
	if level == "" {
		level = database.NotificationLevelInfo
	}
	if !database.IsValidNotificationLevel(level) {
		return database.NotificationUpsert{}, 0, fmt.Errorf("invalid migration notification level %q", def.Level)
	}

	key, err := migrationNotificationKey(candidate, def)
	if err != nil {
		return database.NotificationUpsert{}, 0, err
	}
	var actionURL *string
	if def.ActionURL != "" {
		actionURL = &def.ActionURL
	}
	actionPermissions := aschema.NoPermissions()
	if actionURL != nil {
		actionPermissions = aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"update",
			candidate.Connection.Id.String(),
		)
	}
	metadata := def.Metadata
	if def.Key != "" {
		metadata = map[string]any{}
		for k, v := range def.Metadata {
			metadata[k] = v
		}
		metadata["connector_notice_key"] = def.Key
	}
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
		ActionPermissions: actionPermissions,
		Metadata:          metadata,
	}, migrationNotificationRankForLevel(level), nil
}

func unsetMigrationNotification(
	candidate *connectionMigrationCandidate,
	def migrationNotificationDef,
) error {
	key, err := migrationNotificationKey(candidate, def)
	if err != nil {
		return err
	}
	candidate.NotificationUnsetKeys = appendUniqueString(candidate.NotificationUnsetKeys, key)
	if len(candidate.Notifications) == 1 && candidate.Notifications[0].Key == key {
		candidate.Notifications = nil
		candidate.NotificationKeys = removeString(candidate.NotificationKeys, key)
		candidate.NotificationRank = 0
	}
	return nil
}

func migrationNotificationKey(
	candidate *connectionMigrationCandidate,
	def migrationNotificationDef,
) (string, error) {
	if def.Key == "" {
		return "", errors.New("migration notification key is required")
	}
	return connectionNotificationKey(candidate, "connector_notice:"+def.Key), nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func removeString(values []string, value string) []string {
	result := values[:0]
	for _, existing := range values {
		if existing != value {
			result = append(result, existing)
		}
	}
	return result
}
