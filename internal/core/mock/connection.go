package mock

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

type Connection struct {
	Id                apid.ID
	Namespace         string
	Name              common.ResourceName
	State             database.ConnectionState
	HealthState       database.ConnectionHealthState
	ConnectorId       apid.ID
	ConnectorVersion  uint64
	ActorId           *apid.ID
	ConnectorValue    iface.Connector
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
	Labels            map[string]string
	Annotations       map[string]string
	SetupStep         *cschema.SetupStep
	SetupError        *string
	Configuration     map[string]any
	JavascriptLibrary *apjs.Library
}

func (m *Connection) GetId() apid.ID {
	return m.Id
}

func (m *Connection) GetNamespace() string {
	return m.Namespace
}

func (m *Connection) GetName() common.ResourceName {
	return m.Name
}

func (m *Connection) GetState() database.ConnectionState {
	return m.State
}

func (m *Connection) GetConnectorId() apid.ID {
	return m.ConnectorId
}

func (m *Connection) GetConnectorVersion() uint64 {
	return m.ConnectorVersion
}

func (m *Connection) GetActorId() *apid.ID {
	if m.ActorId == nil {
		return nil
	}
	actorID := *m.ActorId
	return &actorID
}

func (m *Connection) GetCreatedAt() time.Time {
	return m.CreatedAt
}

func (m *Connection) GetUpdatedAt() time.Time {
	return m.UpdatedAt
}

func (m *Connection) GetDeletedAt() *time.Time {
	return m.DeletedAt
}

func (m *Connection) GetLabels() map[string]string {
	return m.Labels
}

func (m *Connection) GetConnector() iface.Connector {
	return m.ConnectorValue
}

func (m *Connection) GetResource(ctx context.Context) (*connectionschema.Connection, error) {
	resource := connectionschema.NewConnection()
	createdAt := m.CreatedAt
	updatedAt := m.UpdatedAt
	resource.Metadata = meta.ObjectMeta{
		ID:          m.Id.String(),
		Name:        m.Name,
		Namespace:   m.Namespace,
		Labels:      m.Labels,
		Annotations: m.Annotations,
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
	resource.Spec.ActorRef = connectionschema.NewActorReference(apid.Nil)
	resource.Spec.Configuration = cloneMap(m.Configuration)
	if m.ActorId != nil {
		resource.Spec.ActorRef = connectionschema.NewActorReference(*m.ActorId)
	}
	if m.ConnectorValue != nil {
		connectorResource := m.ConnectorValue.GetResource()
		resource.Spec.ConnectorRef = meta.NewObjectReference(connectorResource.TypeMeta, connectorResource.Metadata)
	}
	resource.Status = &connectionschema.ConnectionStatus{
		Lifecycle:               connectionschema.ConnectionLifecycleStatus{State: connectionschema.ConnectionState(m.State)},
		Health:                  connectionschema.ConnectionHealthStatus{State: connectionschema.ConnectionHealthState(m.GetHealthState())},
		ConfigurationConfigured: len(m.Configuration) > 0,
	}
	if m.SetupStep != nil || m.SetupError != nil {
		resource.Status.Setup = &connectionschema.ConnectionSetupStatus{Error: m.SetupError}
		if m.SetupStep != nil {
			resource.Status.Setup.StepID = m.SetupStep.String()
		}
	}
	return resource, nil
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (m *Connection) SetState(ctx context.Context, state database.ConnectionState) error {
	m.State = state
	return nil
}

func (m *Connection) GetHealthState() database.ConnectionHealthState {
	if m.HealthState == "" {
		return database.ConnectionHealthStateHealthy
	}
	return m.HealthState
}

func (m *Connection) MarkHealthState(ctx context.Context, state database.ConnectionHealthState, reason string) error {
	m.HealthState = state
	return nil
}

func (m *Connection) GetProbe(probeId string) (iface.Probe, error) {
	return nil, nil
}

func (m *Connection) GetProbes() []iface.Probe {
	return nil
}

func (m *Connection) GetEnabledProbe(ctx context.Context, probeId string) (iface.Probe, error) {
	return m.GetProbe(probeId)
}

func (m *Connection) GetEnabledProbes(ctx context.Context) ([]iface.Probe, error) {
	return m.GetProbes(), nil
}

func (m *Connection) GetAnnotations() map[string]string {
	return m.Annotations
}

func (m *Connection) ProxyRequest(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.ProxyRequest,
) (*iface.ProxyResponse, error) {
	return nil, nil
}

func (m *Connection) ProxyRequestRaw(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.RawProxyRequest,
	w http.ResponseWriter,
) error {
	return nil
}

func (m *Connection) GetSetupStep() *cschema.SetupStep {
	return m.SetupStep
}

func (m *Connection) SetSetupStep(ctx context.Context, setupStep *cschema.SetupStep) error {
	m.SetupStep = setupStep
	return nil
}

func (m *Connection) GetSetupError() *string {
	return m.SetupError
}

func (m *Connection) SetSetupError(ctx context.Context, setupError *string) error {
	m.SetupError = setupError
	return nil
}

func (m *Connection) GetConfiguration(ctx context.Context) (map[string]any, error) {
	return m.Configuration, nil
}

func (m *Connection) SetConfiguration(ctx context.Context, data map[string]any) error {
	m.Configuration = data
	return nil
}

func (m *Connection) GetJavascriptContext(ctx context.Context) (apjs.Context, error) {
	cfg := m.Configuration
	if cfg == nil {
		cfg = map[string]any{}
	}

	labels := m.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	annotations := m.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	return apjs.NewContext(
		m.JavascriptLibrary,
		map[string]any{
			"cfg":         cfg,
			"labels":      labels,
			"annotations": annotations,
		},
	), nil
}

func (m *Connection) GetMustacheContext(ctx context.Context) (map[string]any, error) {
	data := map[string]any{}

	if m.Configuration != nil {
		data["cfg"] = m.Configuration
	}

	if len(m.Labels) > 0 {
		data["labels"] = m.Labels
	}

	if len(m.Annotations) > 0 {
		data["annotations"] = m.Annotations
	}

	return data, nil
}

func (m *Connection) SubmitForm(ctx context.Context, req iface.SubmitConnectionRequest) (iface.ConnectionSetupResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) GetCurrentSetupStepResponse(ctx context.Context, returnToUrl string) (iface.ConnectionSetupResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) Reconfigure(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) CancelSetup(ctx context.Context) error {
	if m.State != database.ConnectionStateConfigured {
		return fmt.Errorf("connection is not in a state that can cancel setup")
	}
	m.SetupStep = nil
	m.SetupError = nil
	return nil
}

func (m *Connection) HandleCredentialsEstablished(ctx context.Context) (iface.PostAuthOutcome, error) {
	return iface.PostAuthOutcome{SetupPending: false}, nil
}

func (m *Connection) HandleAuthFailed(ctx context.Context, authErr error) error {
	msg := authErr.Error()
	m.SetupError = &msg
	failedStep := cschema.SetupStepAuthFailed
	m.SetupStep = &failedStep
	return nil
}

var _ iface.Connection = (*Connection)(nil)

type ConnectionMatcher struct {
	ExpectedId apid.ID
}

func (m ConnectionMatcher) Matches(x interface{}) bool {
	c, ok := x.(iface.Connection)
	if !ok {
		return false
	}

	return c.GetId() == m.ExpectedId
}

func (m ConnectionMatcher) String() string {
	return fmt.Sprintf("is Connection with ID=%s", m.ExpectedId)
}
