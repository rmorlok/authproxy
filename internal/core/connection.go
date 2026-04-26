package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

// Connection is a wrapper for the lower level database equivalent that handles wiring up logic specified in this
// connection's connector version.
type connection struct {
	database.Connection

	s      *service
	cv     *ConnectorVersion
	logger *slog.Logger

	proxyImplOnce sync.Once
	proxyImpl     iface.Proxy
	proxyImplErr  error
}

func wrapConnection(c *database.Connection, cv *ConnectorVersion, s *service) *connection {
	return &connection{
		Connection: *c,
		s:          s,
		cv:         cv,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(c.Namespace).
			WithConnectionId(c.Id).
			WithConnectorId(cv.Id).
			WithConnectorVersion(cv.Version).
			Build(),
	}
}

func (c *connection) GetId() apid.ID {
	return c.Id
}

func (c *connection) GetNamespace() string {
	return c.Namespace
}

func (c *connection) GetState() database.ConnectionState {
	return c.State
}

func (c *connection) GetConnectorId() apid.ID {
	return c.ConnectorId
}

func (c *connection) GetConnectorVersion() uint64 {
	return c.ConnectorVersion
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

func (c *connection) GetSetupStep() *string {
	return c.SetupStep
}

func (c *connection) GetConnectorVersionEntity() iface.ConnectorVersion {
	return c.cv
}

func (c *connection) Logger() *slog.Logger {
	return c.logger
}

func (c *connection) SetSetupStep(ctx context.Context, setupStep *cschema.SetupStep) error {
	var setupStepStr *string
	if setupStep != nil {
		setupStepStr = util.ToPtr(setupStep.String())
	}

	if err := c.s.db.SetConnectionSetupStep(ctx, c.Id, setupStepStr); err != nil {
		return err
	}
	c.SetupStep = setupStepStr
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
	if c.EncryptedConfiguration == nil || c.EncryptedConfiguration.IsZero() {
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

	return result, nil
}

func (c *connection) SetConfiguration(ctx context.Context, data map[string]any) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal connection configuration: %w", err)
	}

	ef, err := c.s.encrypt.EncryptStringForNamespace(ctx, c.Namespace, string(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to encrypt connection configuration: %w", err)
	}

	if err := c.s.db.SetConnectionEncryptedConfiguration(ctx, c.Id, &ef); err != nil {
		return err
	}

	c.EncryptedConfiguration = &ef
	return nil
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

func (c *connection) GetRateLimitConfig() *connectors.RateLimiting {
	def := c.cv.GetDefinition()
	if def == nil {
		return nil
	}
	return def.RateLimiting
}

var _ iface.Connection = (*connection)(nil)
var _ aplog.HasLogger = (*connection)(nil)
var _ httpf.RateLimitConfigProvider = (*connection)(nil)
