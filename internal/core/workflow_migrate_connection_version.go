package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameMigrateConnectionVersionV1 = "core.connection.migrate_version.v1"

	ActivityNameMigrateConnectionVersionApplyV1 = "core.connection.migrate_version.apply.v1"

	connectionMigrationNotificationSource = "connector_migration"
)

type migrateConnectionVersionWorkflowInputV1 struct {
	ConnectionID  apid.ID       `json:"connection_id"`
	TargetVersion uint64        `json:"target_version"`
	Timeout       time.Duration `json:"timeout"`
}

func migrateConnectionVersionWorkflowV1(ctx wflib.Context, input migrateConnectionVersionWorkflowInputV1) error {
	activityCtx, cancelActivities := wflib.WithCancel(ctx)
	defer cancelActivities()

	timerCtx, cancelTimer := wflib.WithCancel(ctx)
	timer := wflib.ScheduleTimer(timerCtx, input.Timeout, wflib.WithTimerName("migration-timeout"))
	defer cancelTimer()

	applyFuture := wflib.ExecuteActivity[any](
		activityCtx,
		wflib.DefaultActivityOptions,
		ActivityNameMigrateConnectionVersionApplyV1,
		input.ConnectionID,
		input.TargetVersion,
	)

	var applyErr error
	timedOut := false
	wflib.Select(ctx,
		wflib.Await(timer, func(_ wflib.Context, _ wflib.Future[any]) {
			timedOut = true
			cancelActivities()
		}),
		wflib.Await(applyFuture, func(ctx wflib.Context, future wflib.Future[any]) {
			_, applyErr = future.Get(ctx)
		}),
	)
	if timedOut {
		return fmt.Errorf("connection version migration timed out")
	}
	if applyErr != nil {
		return applyErr
	}

	cancelTimer()
	return nil
}

func (s *service) registerMigrateConnectionVersionWorkflow(worker workflowRegistrar) error {
	if err := worker.RegisterWorkflow(
		migrateConnectionVersionWorkflowV1,
		registry.WithName(WorkflowNameMigrateConnectionVersionV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.applyMigrateConnectionVersionV1,
		registry.WithName(ActivityNameMigrateConnectionVersionApplyV1),
	)
}

func migrateConnectionVersionWorkflowInstanceID(connectionID apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameMigrateConnectionVersionV1, connectionID)
}

func (s *service) startMigrateConnectionVersionWorkflow(
	ctx context.Context,
	connectionID apid.ID,
	opts iface.ConnectionMigrationOptions,
) (*wflib.Instance, error) {
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: migrateConnectionVersionWorkflowInstanceID(connectionID),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameMigrateConnectionVersionV1, migrateConnectionVersionWorkflowInputV1{
		ConnectionID:  connectionID,
		TargetVersion: opts.TargetVersion,
		Timeout:       opts.Timeout,
	})
}

type migrationHookPatch struct {
	Config        migrationAnyPatch          `json:"config"`
	Labels        migrationStringPatch       `json:"labels"`
	Annotations   migrationStringPatch       `json:"annotations"`
	Notifications []migrationNotificationDef `json:"notifications"`
}

type migrationAnyPatch struct {
	Set   map[string]any `json:"set"`
	Unset []string       `json:"unset"`
}

type migrationStringPatch struct {
	Set   map[string]string `json:"set"`
	Unset []string          `json:"unset"`
}

type migrationNotificationDef struct {
	Key       string         `json:"key"`
	Level     string         `json:"level"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	ActionURL string         `json:"action_url"`
	Metadata  map[string]any `json:"metadata"`
}

type connectionMigrationCandidate struct {
	Connection       *connection
	Target           *ConnectorVersion
	Config           map[string]any
	UserLabels       map[string]string
	Annotations      map[string]string
	SetupStep        *cschema.SetupStep
	SetupError       *string
	HealthState      database.ConnectionHealthState
	RefreshAuth      bool
	ProbeIdsToRun    []string
	Notifications    []database.NotificationUpsert
	NotificationKeys []string
}

func (s *service) applyMigrateConnectionVersionV1(ctx context.Context, connectionID apid.ID, targetVersion uint64) error {
	logger := s.logger.With(
		"workflow", WorkflowNameMigrateConnectionVersionV1,
		"activity", ActivityNameMigrateConnectionVersionApplyV1,
		"connection_id", connectionID,
		"target_version", targetVersion,
	)
	logger.Info("connection version migration apply activity started")
	defer logger.Info("connection version migration apply activity completed")

	candidate, err := s.buildConnectionMigrationCandidate(ctx, connectionID, targetVersion)
	if err != nil {
		return err
	}

	encryptedConfig, err := s.encryptMigrationConfig(ctx, candidate.Connection.Namespace, candidate.Config)
	if err != nil {
		return err
	}

	health := candidate.HealthState
	if health == "" {
		health = candidate.Connection.GetHealthState()
	}
	updated, err := s.db.UpdateConnectionForVersionMigration(ctx, database.ConnectionVersionMigrationUpdate{
		Id:                     connectionID,
		ConnectorId:            candidate.Connection.ConnectorId,
		ConnectorVersion:       targetVersion,
		EncryptedConfiguration: encryptedConfig,
		UserLabels:             candidate.UserLabels,
		Annotations:            candidate.Annotations,
		SetupStep:              candidate.SetupStep,
		SetupError:             candidate.SetupError,
		HealthState:            &health,
	})
	if err != nil {
		return err
	}

	if candidate.RefreshAuth {
		if err := s.refreshAuthAfterConnectionMigration(ctx, updated, candidate); err != nil {
			candidate.HealthState = database.ConnectionHealthStateUnhealthy
			s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
				"Connection requires re-authentication",
				"The target connector version changed OAuth settings and AuthProxy could not refresh credentials automatically.",
				fmt.Sprintf("target:%d:oauth:refresh_failed", candidate.Target.Version),
				"reauth",
				map[string]any{
					"connector_id":     candidate.Connection.ConnectorId.String(),
					"source_version":   candidate.Connection.ConnectorVersion,
					"target_version":   candidate.Target.Version,
					"migration_event":  "oauth_refresh_failed",
					"refresh_error":    err.Error(),
					"requires_reauth":  true,
					"auth_method_type": string(cschema.AuthTypeOAuth2),
				})
			if markErr := s.db.SetConnectionHealthState(ctx, connectionID, database.ConnectionHealthStateUnhealthy); markErr != nil {
				return markErr
			}
		}
	}
	if len(candidate.ProbeIdsToRun) > 0 && candidate.SetupStep == nil && candidate.HealthState != database.ConnectionHealthStateUnhealthy {
		for _, probeID := range candidate.ProbeIdsToRun {
			if err := s.RunProbe(ctx, connectionID, probeID); err != nil {
				logger.Warn("new target probe failed after migration", "probe_id", probeID, "error", err)
			}
		}
	}

	if err := s.db.ResolveNotificationsForResource(ctx, "connection", connectionID, connectionMigrationNotificationSource, candidate.NotificationKeys); err != nil {
		return err
	}
	for _, notification := range candidate.Notifications {
		if _, err := s.db.UpsertNotification(ctx, notification); err != nil {
			return err
		}
	}

	logger.Info(
		"connection version migration applied",
		"source_version", candidate.Connection.ConnectorVersion,
		"target_version", updated.ConnectorVersion,
	)
	return nil
}

func (s *service) buildConnectionMigrationCandidate(ctx context.Context, connectionID apid.ID, targetVersion uint64) (*connectionMigrationCandidate, error) {
	conn, err := s.getConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	if conn.ConnectorVersion == targetVersion {
		return nil, fmt.Errorf("connection is already on connector version %d", targetVersion)
	}

	target, err := s.getConnectorVersion(ctx, conn.ConnectorId, targetVersion)
	if err != nil {
		return nil, err
	}
	if target.State != database.ConnectorVersionStatePrimary && target.State != database.ConnectorVersionStateActive {
		return nil, fmt.Errorf("target connector version must be primary or active")
	}

	cfg, err := conn.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	userLabels, _ := database.SplitUserAndApxyLabels(conn.Labels)
	annotations := map[string]string{}
	for k, v := range conn.Annotations {
		annotations[k] = v
	}

	candidate := &connectionMigrationCandidate{
		Connection:  conn,
		Target:      target,
		Config:      cfg,
		UserLabels:  map[string]string(userLabels),
		Annotations: annotations,
		SetupStep:   conn.SetupStep,
		SetupError:  conn.SetupError,
		HealthState: conn.GetHealthState(),
	}

	versions, err := s.migrationVersionPath(ctx, conn.ConnectorId, conn.ConnectorVersion, targetVersion)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if err := s.applyMigrationHookForVersion(ctx, candidate, version, conn.ConnectorVersion, targetVersion); err != nil {
			return nil, err
		}
	}
	if err := s.applyAuthMigrationAnalysis(candidate); err != nil {
		return nil, err
	}
	s.applyProbeMigrationAnalysis(candidate)
	if err := s.applySetupFlowMigrationAnalysis(candidate); err != nil {
		return nil, err
	}

	return candidate, nil
}

func (s *service) applyProbeMigrationAnalysis(candidate *connectionMigrationCandidate) {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if sourceDef == nil || targetDef == nil {
		return
	}
	sourceProbeIDs := map[string]bool{}
	for _, probe := range sourceDef.Probes {
		sourceProbeIDs[probe.Id] = true
	}
	for _, probe := range targetDef.Probes {
		if probe.Id == "" || sourceProbeIDs[probe.Id] {
			continue
		}
		candidate.ProbeIdsToRun = append(candidate.ProbeIdsToRun, probe.Id)
		s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			fmt.Sprintf("New connection health probe %q will run", probe.Id),
			"The target connector version adds a new health probe. AuthProxy will run it once after migration if no user action is required.",
			fmt.Sprintf("target:%d:probe:%s:added", candidate.Target.Version, probe.Id),
			"", map[string]any{
				"connector_id":    candidate.Connection.ConnectorId.String(),
				"source_version":  candidate.Connection.ConnectorVersion,
				"target_version":  candidate.Target.Version,
				"probe_id":        probe.Id,
				"migration_event": "probe_added",
			})
	}
}

func (s *service) applyAuthMigrationAnalysis(candidate *connectionMigrationCandidate) error {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if sourceDef == nil || targetDef == nil || targetDef.Auth == nil {
		return nil
	}
	if targetDef.Auth.GetType() != cschema.AuthTypeOAuth2 {
		return nil
	}

	sourceJSON, err := json.Marshal(sourceDef.Auth)
	if err != nil {
		return err
	}
	targetJSON, err := json.Marshal(targetDef.Auth)
	if err != nil {
		return err
	}
	if string(sourceJSON) != string(targetJSON) {
		candidate.RefreshAuth = true
		s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			"Connection credentials will be refreshed",
			"The target connector version changes OAuth settings, so AuthProxy will refresh credentials after migration.",
			fmt.Sprintf("target:%d:oauth:refresh_required", candidate.Target.Version),
			"", map[string]any{
				"connector_id":     candidate.Connection.ConnectorId.String(),
				"source_version":   candidate.Connection.ConnectorVersion,
				"target_version":   candidate.Target.Version,
				"migration_event":  "oauth_refresh_required",
				"auth_method_type": string(cschema.AuthTypeOAuth2),
			})
	}
	return nil
}

func (s *service) refreshAuthAfterConnectionMigration(ctx context.Context, updated *database.Connection, candidate *connectionMigrationCandidate) error {
	conn, err := s.getConnectionForDb(ctx, updated)
	if err != nil {
		return err
	}
	factory := s.getAuthMethodFactory(candidate.Target.GetDefinition())
	if factory == nil {
		return fmt.Errorf("auth method factory is not configured")
	}
	authenticator := factory.NewAuthenticator(conn)
	if authenticator == nil {
		return fmt.Errorf("authenticator is not configured")
	}
	return authenticator.RecoverFrom401(ctx)
}

func (s *service) migrationVersionPath(ctx context.Context, connectorID apid.ID, sourceVersion, targetVersion uint64) ([]*ConnectorVersion, error) {
	var versions []*ConnectorVersion
	if targetVersion > sourceVersion {
		for v := sourceVersion + 1; v <= targetVersion; v++ {
			cv, err := s.getConnectorVersion(ctx, connectorID, v)
			if err != nil {
				return nil, err
			}
			versions = append(versions, cv)
		}
		return versions, nil
	}

	for v := sourceVersion; v > targetVersion; v-- {
		cv, err := s.getConnectorVersion(ctx, connectorID, v)
		if err != nil {
			return nil, err
		}
		versions = append(versions, cv)
	}
	return versions, nil
}

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

func (s *service) applySetupFlowMigrationAnalysis(candidate *connectionMigrationCandidate) error {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if targetDef == nil || targetDef.SetupFlow == nil {
		return nil
	}

	sourceFields := map[string]bool{}
	if sourceDef != nil && sourceDef.SetupFlow != nil {
		sourceFields = sourceDef.SetupFlow.AllConfigFieldNames()
	}

	if targetDef.SetupFlow.Preconnect != nil {
		fields, err := targetDef.SetupFlow.Preconnect.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target preconnect setup fields: %w", err)
		}
		if err := s.applySetupFieldMigrationAnalysis(candidate, "preconnect", fields, sourceFields); err != nil {
			return err
		}
	}

	if targetDef.SetupFlow.Configure != nil {
		fields, err := targetDef.SetupFlow.Configure.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target configure setup fields: %w", err)
		}
		if err := s.applySetupFieldMigrationAnalysis(candidate, "configure", fields, sourceFields); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) applySetupFieldMigrationAnalysis(
	candidate *connectionMigrationCandidate,
	phase string,
	fields []cschema.SetupField,
	sourceFields map[string]bool,
) error {
	for _, field := range fields {
		if sourceFields[field.Name] {
			continue
		}
		if _, ok := candidate.Config[field.Name]; ok {
			continue
		}

		metadata := map[string]any{
			"connector_id":    candidate.Connection.ConnectorId.String(),
			"source_version":  candidate.Connection.ConnectorVersion,
			"target_version":  candidate.Target.Version,
			"setup_phase":     phase,
			"setup_step_id":   field.StepId,
			"config_field":    field.Name,
			"migration_event": "setup_field_added",
		}

		if field.HasDefault {
			candidate.Config[field.Name] = field.Default
			s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
				fmt.Sprintf("Connection setting %q defaulted", field.Name),
				fmt.Sprintf("The connector version migration added the new %s setting %q using the connector default.", phase, field.Name),
				fmt.Sprintf("target:%d:setup:%s:%s:default", candidate.Target.Version, phase, field.Name),
				"", metadata)
			continue
		}

		if field.Required {
			if phase == "preconnect" {
				candidate.HealthState = database.ConnectionHealthStateUnhealthy
				s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
					"Connection requires re-authentication",
					fmt.Sprintf("The target connector version requires new preconnect setting %q before credentials can be refreshed.", field.Name),
					fmt.Sprintf("target:%d:setup:%s:%s:required", candidate.Target.Version, phase, field.Name),
					"reauth", metadata)
				continue
			}

			if candidate.SetupStep == nil {
				step, err := cschema.NewSetupStep(field.StepId)
				if err != nil {
					return err
				}
				candidate.SetupStep = &step
			}
			s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
				"Connection requires configuration",
				fmt.Sprintf("The target connector version requires new configuration setting %q.", field.Name),
				fmt.Sprintf("target:%d:setup:%s:%s:required", candidate.Target.Version, phase, field.Name),
				"configure", metadata)
			continue
		}

		s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			fmt.Sprintf("New optional connection setting %q is available", field.Name),
			fmt.Sprintf("The target connector version adds optional %s setting %q.", phase, field.Name),
			fmt.Sprintf("target:%d:setup:%s:%s:optional", candidate.Target.Version, phase, field.Name),
			"configure", metadata)
	}
	return nil
}

func (s *service) addMigrationSystemNotification(
	candidate *connectionMigrationCandidate,
	level database.NotificationLevel,
	title string,
	message string,
	keyPart string,
	action string,
	metadata map[string]any,
) {
	key := fmt.Sprintf("%s:%s:%s", connectionMigrationNotificationSource, candidate.Connection.Id, keyPart)
	source := connectionMigrationNotificationSource
	actionURL := ""
	if action != "" {
		actionURL = fmt.Sprintf("/connections/%s?action=%s", candidate.Connection.Id, action)
	}
	var actionURLPtr *string
	if actionURL != "" {
		actionURLPtr = &actionURL
	}
	actionPermissions := aschema.NoPermissions()
	if action != "" {
		actionPermissions = aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"update",
			candidate.Connection.Id.String(),
		)
	}
	upsert := database.NotificationUpsert{
		Key:          key,
		Level:        level,
		ResourceType: "connection",
		ResourceId:   candidate.Connection.Id,
		Namespace:    candidate.Connection.Namespace,
		Title:        title,
		Message:      message,
		ActionUrl:    actionURLPtr,
		ViewPermissions: aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"get",
			candidate.Connection.Id.String(),
		),
		ActionPermissions: actionPermissions,
		Source:            &source,
		Metadata:          metadata,
	}
	candidate.Notifications = append(candidate.Notifications, upsert)
	candidate.NotificationKeys = append(candidate.NotificationKeys, upsert.Key)
}

func (s *service) encryptMigrationConfig(ctx context.Context, namespace string, cfg map[string]any) (*encfield.EncryptedField, error) {
	if cfg == nil {
		return nil, nil
	}
	jsonBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal migrated connection configuration: %w", err)
	}
	encrypted, err := s.encrypt.EncryptStringForNamespace(ctx, namespace, string(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("encrypt migrated connection configuration: %w", err)
	}
	return &encrypted, nil
}
