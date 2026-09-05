package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// Connection is a wrapper for the lower level database equivalent that handles wiring up logic specified in this
// connection's connector version.
type connection struct {
	database.Connection

	s         *service
	connector *Connector
	logger    *slog.Logger

	configMu     sync.Mutex
	configLoaded bool
	configCache  map[string]any

	proxyImplOnce sync.Once
	proxyImpl     iface.Proxy
	proxyImplErr  error
}

func wrapConnection(dbConnection *database.Connection, c *Connector, s *service) *connection {
	return &connection{
		Connection: *dbConnection,
		s:          s,
		connector:  c,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(dbConnection.Namespace).
			WithConnectionId(dbConnection.Id).
			WithConnectorId(c.Id).
			WithConnectorVersion(c.Version).
			Build(),
	}
}

func (c *connection) GetId() apid.ID {
	return c.Id
}

func (c *connection) GetNamespace() string {
	return c.Namespace
}

func (c *connection) GetName() scommon.ResourceName {
	return c.Name
}

func (c *connection) GetState() database.ConnectionState {
	return c.State
}

func (c *connection) GetHealthState() database.ConnectionHealthState {
	if c.HealthState == "" {
		return database.ConnectionHealthStateHealthy
	}
	return c.HealthState
}

func (c *connection) GetConnectorId() apid.ID {
	return c.ConnectorId
}

func (c *connection) GetConnectorVersion() uint64 {
	return c.ConnectorVersion
}

func (c *connection) GetActorId() *apid.ID {
	if c.ActorId == nil {
		return nil
	}
	actorID := *c.ActorId
	return &actorID
}

func (c *connection) GetCreatedAt() time.Time {
	return c.CreatedAt
}

func (c *connection) GetUpdatedAt() time.Time {
	return c.UpdatedAt
}

func (c *connection) GetDeletedAt() *time.Time {
	return c.DeletedAt
}

func (c *connection) GetLabels() map[string]string {
	return c.Labels
}

func (c *connection) GetAnnotations() map[string]string {
	return c.Annotations
}

func (c *connection) GetSetupStep() *cschema.SetupStep {
	return c.SetupStep
}

func (c *connection) GetConnector() iface.Connector {
	return c.connector
}

func (c *connection) GetResource(ctx context.Context) (*connectionschema.Connection, error) {
	createdAt := c.CreatedAt
	updatedAt := c.UpdatedAt
	healthState := c.GetHealthState()
	configuration, err := c.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	configuration, err = connectionschema.RedactConfiguration(configuration)
	if err != nil {
		return nil, err
	}

	resource := &connectionschema.Connection{
		TypeMeta: meta.NewTypeMeta(connectionschema.ConnectionKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          c.Id.String(),
			Name:        c.Name,
			Namespace:   c.Namespace,
			Labels:      maps.Clone(map[string]string(c.Labels)),
			Annotations: maps.Clone(map[string]string(c.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: connectionschema.ConnectionSpec{
			ConnectorRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       cschema.ConnectorKind,
				ID:         c.connector.GetId().String(),
				Name:       c.connector.GetName(),
				Namespace:  c.connector.GetNamespace(),
				Generation: c.connector.GetVersion(),
			},
			ActorRef:      connectionschema.NewActorReference(apid.Nil),
			Configuration: configuration,
		},
		Status: &connectionschema.ConnectionStatus{
			Lifecycle: connectionschema.ConnectionLifecycleStatus{
				State: connectionschema.ConnectionState(c.State),
			},
			Health: connectionschema.ConnectionHealthStatus{
				State: connectionschema.ConnectionHealthState(healthState),
			},
			ConfigurationConfigured: c.EncryptedConfiguration != nil && !c.EncryptedConfiguration.IsZero(),
		},
	}

	if c.ActorId != nil {
		resource.Spec.ActorRef = connectionschema.NewActorReference(*c.ActorId)
	}
	if c.SetupStep != nil || c.SetupError != nil {
		resource.Status.Setup = &connectionschema.ConnectionSetupStatus{Error: c.GetSetupError()}
		if c.SetupStep != nil {
			resource.Status.Setup.StepID = c.SetupStep.String()
		}
	}

	return resource, nil
}

func (c *connection) GetJavascriptContext(ctx context.Context) (apjs.Context, error) {
	jsLib, err := c.connector.getJavascriptLibrary()
	if err != nil {
		return apjs.Context{}, err
	}

	cfg, err := c.GetConfiguration(ctx)
	if err != nil {
		return apjs.Context{}, fmt.Errorf("get connection configuration: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	labels := c.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	annotations := c.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	return apjs.NewContext(
		jsLib,
		map[string]any{
			"cfg":         cfg,
			"labels":      labels,
			"annotations": annotations,
		},
	), nil
}

func (c *connection) Logger() *slog.Logger {
	return c.logger
}

func (c *connection) SetSetupStep(ctx context.Context, setupStep *cschema.SetupStep) error {
	if err := c.s.db.SetConnectionSetupStep(ctx, c.Id, setupStep); err != nil {
		return err
	}
	c.SetupStep = setupStep
	return nil
}

func (c *connection) GetSetupError() *string {
	return c.SetupError
}

func (c *connection) SetSetupError(ctx context.Context, setupError *string) error {
	if err := c.s.db.SetConnectionSetupError(ctx, c.Id, setupError); err != nil {
		return err
	}
	c.SetupError = setupError
	return nil
}

func (c *connection) GetConfiguration(ctx context.Context) (map[string]any, error) {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.configLoaded {
		return cloneConfiguration(c.configCache), nil
	}

	if c.EncryptedConfiguration == nil || c.EncryptedConfiguration.IsZero() {
		c.configLoaded = true
		c.configCache = nil
		return nil, nil
	}

	decrypted, err := c.s.encrypt.DecryptString(ctx, *c.EncryptedConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt connection configuration: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(decrypted), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connection configuration: %w", err)
	}

	c.configLoaded = true
	c.configCache = result

	return cloneConfiguration(c.configCache), nil
}

func (c *connection) SetConfiguration(ctx context.Context, data map[string]any) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal connection configuration: %w", err)
	}

	var cached map[string]any
	if err := json.Unmarshal(jsonBytes, &cached); err != nil {
		return fmt.Errorf("failed to normalize connection configuration: %w", err)
	}

	ef, err := c.s.encrypt.EncryptStringForNamespace(ctx, c.Namespace, string(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to encrypt connection configuration: %w", err)
	}

	if err := c.s.db.SetConnectionEncryptedConfiguration(ctx, c.Id, &ef); err != nil {
		return err
	}

	c.EncryptedConfiguration = &ef
	c.configCache = cached
	c.configLoaded = true
	return nil
}

func cloneConfiguration(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}

	cloned := make(map[string]any, len(cfg))
	for key, value := range cfg {
		cloned[key] = cloneConfigurationValue(value)
	}
	return cloned
}

func cloneConfigurationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneConfiguration(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneConfigurationValue(item)
		}
		return cloned
	default:
		return typed
	}
}

func (c *connection) GetMustacheContext(ctx context.Context) (map[string]any, error) {
	data := map[string]any{}

	cfg, err := c.GetConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection configuration for mustache context: %w", err)
	}
	if cfg != nil {
		data["cfg"] = cfg
	}

	if labels := c.GetLabels(); len(labels) > 0 {
		data["labels"] = labels
	}

	if annotations := c.GetAnnotations(); len(annotations) > 0 {
		data["annotations"] = annotations
	}

	return data, nil
}

func (c *connection) GetRateLimitConfig() *cschema.RateLimiting {
	def := c.connector.GetDefinition()
	if def == nil {
		return nil
	}
	return def.RateLimiting
}

// PropagateTraceContext returns the per-connector override for outbound W3C
// trace context injection. nil means "use the global default" from the
// telemetry config block.
func (c *connection) PropagateTraceContext() *bool {
	def := c.connector.GetDefinition()
	if def == nil || def.Telemetry == nil {
		return nil
	}
	return def.Telemetry.PropagateTraceContext
}

var _ iface.Connection = (*connection)(nil)
var _ aplog.HasLogger = (*connection)(nil)
var _ httpf.RateLimitConfigProvider = (*connection)(nil)
var _ httpf.TracePropagationProvider = (*connection)(nil)
