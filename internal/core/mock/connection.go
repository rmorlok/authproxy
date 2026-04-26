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
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

type Connection struct {
	Id               apid.ID
	Namespace        string
	State            database.ConnectionState
	ConnectorId      apid.ID
	ConnectorVersion uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
	Labels           map[string]string
	Annotations      map[string]string
	SetupStep        *cschema.SetupStep
	SetupError       *string
	Configuration    map[string]any
}

func (m *Connection) GetId() apid.ID {
	return m.Id
}

func (m *Connection) GetNamespace() string {
	return m.Namespace
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

func (m *Connection) GetConnectorVersionEntity() iface.ConnectorVersion {
	return nil
}

func (m *Connection) SetState(ctx context.Context, state database.ConnectionState) error {
	m.State = state
	return nil
}

func (m *Connection) GetProbe(probeId string) (iface.Probe, error) {
	return nil, nil
}

func (m *Connection) GetProbes() []iface.Probe {
	return nil
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
	req *iface.ProxyRequest,
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

func (m *Connection) GetCurrentSetupStepResponse(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) Reconfigure(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *Connection) CancelSetup(ctx context.Context) error {
	if m.State != database.ConnectionStateReady {
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
