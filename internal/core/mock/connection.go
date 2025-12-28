package mock

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/request_log"
)

type Connection struct {
	Id               uuid.UUID
	Namespace        string
	State            database.ConnectionState
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func (m *Connection) GetId() uuid.UUID {
	return m.Id
}

func (m *Connection) GetNamespace() string {
	return m.Namespace
}

func (m *Connection) GetState() database.ConnectionState {
	return m.State
}

func (m *Connection) GetConnectorId() uuid.UUID {
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
	reqType request_log.RequestType,
	req *iface.ProxyRequest,
) (*iface.ProxyResponse, error) {
	return nil, nil
}

func (m *Connection) ProxyRequestRaw(
	ctx context.Context,
	reqType request_log.RequestType,
	req *iface.ProxyRequest,
	w http.ResponseWriter,
) error {
	return nil
}

var _ iface.Connection = (*Connection)(nil)

type ConnectionMatcher struct {
	ExpectedId uuid.UUID
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
