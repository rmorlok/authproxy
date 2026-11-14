package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// applyMigrationHookForGeneration applies the javascript upgrade/downgrade hook,
// if one exists, for a connection generation migration. This may modify the
// candidate by changing the config, user labels, and annotations, and/or
// adding/removing notifications. Authored code receives copied inputs and a finite
// execution policy; only its detached patch changes the candidate. Definition
// loading retains infrastructure errors, while authored execution errors are safe.
func (s *service) applyMigrationHookForGeneration(
	ctx context.Context,
	candidate *connectionMigrationCandidate,
	generation *Connector,
	sourceGeneration,
	targetGeneration uint64,
) (err error) {
	if ctx == nil {
		return fmt.Errorf("evaluate migration hook for generation %d: %w", generation.Generation, jsruntime.ErrInvalidInput)
	}
	def, err := generation.getDefinitionWithContext(ctx)
	if err != nil {
		return fmt.Errorf("load migration hook definition for generation %d: %w", generation.Generation, err)
	}
	if def == nil || def.Migrations == nil {
		return nil
	}

	var hook *cschema.MigrationHook
	if targetGeneration > sourceGeneration {
		hook = def.Migrations.Up
	} else {
		hook = def.Migrations.Down
	}
	if hook == nil || hook.Javascript == "" {
		return nil
	}
	defer func() {
		// Only host-owned diagnostic fields cross into logs. Error messages,
		// output paths, and source snippets can contain connection secrets.
		diagnostic, ok := apjs.Diagnostic(err)
		if !ok {
			return
		}
		attrs := []any{
			"operation", "migrate_connection_generation",
			"connection_id", candidate.Connection.Id,
			"connector_id", generation.Id,
			"generation", generation.Generation,
			"source_generation", sourceGeneration,
			"target_generation", targetGeneration,
			"phase", diagnostic.Phase,
			"category", diagnostic.Category,
		}
		if diagnostic.Source != "" {
			attrs = append(attrs, "source", diagnostic.Source)
		}
		if diagnostic.Line > 0 && diagnostic.Column > 0 {
			attrs = append(attrs, "line", diagnostic.Line, "column", diagnostic.Column)
		}
		s.logger.WarnContext(ctx, "migration hook JavaScript failed", attrs...)
	}()

	limits := apjs.ConnectorMigrationLimits()
	// The shared loader bounds library and hook source before compiling or
	// reusing the library, including when this caller has a tighter policy.
	jsLib, err := generation.getBoundedJavascriptLibrary(ctx, hook.Javascript, limits)
	if err != nil {
		return fmt.Errorf("evaluate migration hook for generation %d: %w", generation.Generation, err)
	}
	result, err := apjs.NewContext(jsLib, map[string]any{
		"cfg":         candidate.Config,
		"labels":      candidate.UserLabels,
		"annotations": candidate.Annotations,
	}).EvaluateObjectJSONWithContext(ctx, hook.Javascript, limits)
	if err != nil {
		return fmt.Errorf("evaluate migration hook for generation %d: %w", generation.Generation, err)
	}

	patch, err := decodeMigrationHookPatch(result)
	// Decoding is bounded by the detached JSON size. Honor cancellation that
	// arrived during this Go work before applying any returned patch operations.
	if contextErr := ctx.Err(); contextErr != nil {
		return fmt.Errorf("decode migration hook for generation %d: %w", generation.Generation, contextErr)
	}
	if err != nil {
		return fmt.Errorf("decode migration hook for generation %d: %w", generation.Generation, err)
	}
	if err := applyMigrationHookPatch(candidate, patch); err != nil {
		return fmt.Errorf("apply migration hook for generation %d: %w", generation.Generation, err)
	}
	return nil
}

// decodeMigrationHookPatch decodes bounded, detached JSON directly into the
// migration patch contract. Configuration values keep the existing JSON number
// representation; no live JavaScript values or intermediate exported map remain.
func decodeMigrationHookPatch(raw json.RawMessage) (migrationHookPatch, error) {
	if len(raw) == 0 {
		return migrationHookPatch{}, nil
	}

	var patch migrationHookPatch
	if err := json.Unmarshal(raw, &patch); err != nil {
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
		if err := smeta.ValidateUserLabelKey(key); err != nil {
			return err
		}
		if err := smeta.ValidateLabelValue(value); err != nil {
			return err
		}
		candidate.UserLabels[key] = value
	}
	for _, key := range patch.Labels.Unset {
		if err := smeta.ValidateUserLabelKey(key); err != nil {
			return err
		}
		delete(candidate.UserLabels, key)
	}

	// Update the annotations
	for key, value := range patch.Annotations.Set {
		if err := smeta.ValidateAnnotationKey(key); err != nil {
			return err
		}
		candidate.Annotations[key] = value
	}
	for _, key := range patch.Annotations.Unset {
		if err := smeta.ValidateAnnotationKey(key); err != nil {
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
