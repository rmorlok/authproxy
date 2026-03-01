package mock

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
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
