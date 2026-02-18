package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

type ConnectorVersion struct {
	Id         uuid.UUID
	Namespace  string
	Version    uint64
	State      database.ConnectorVersionState
	Type       string
	Hash       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Labels     map[string]string
	Definition *cschema.Connector
}

func (m *ConnectorVersion) GetId() uuid.UUID {
	return m.Id
}

func (m *ConnectorVersion) GetNamespace() string {
	return m.Namespace
}

func (m *ConnectorVersion) GetVersion() uint64 {
	return m.Version
}

func (m *ConnectorVersion) GetState() database.ConnectorVersionState {
	return m.State
}

func (m *ConnectorVersion) GetType() string {
	return m.Type
}

func (m *ConnectorVersion) GetHash() string {
	return m.Hash
}

func (m *ConnectorVersion) GetCreatedAt() time.Time {
	return m.CreatedAt
}

func (m *ConnectorVersion) GetUpdatedAt() time.Time {
	return m.UpdatedAt
}

func (m *ConnectorVersion) GetLabels() map[string]string {
	return m.Labels
}

func (m *ConnectorVersion) GetDefinition() *cschema.Connector {
	return m.Definition
}

func (m *ConnectorVersion) SetState(_ context.Context, state database.ConnectorVersionState) error {
	m.State = state
	return nil
}

var _ iface.ConnectorVersion = (*ConnectorVersion)(nil)

type ConnectorVersionMatcher struct {
	ExpectedId      uuid.UUID
	ExpectedVersion uint64
}

func (m ConnectorVersionMatcher) Matches(x interface{}) bool {
	cv, ok := x.(iface.ConnectorVersion)
	if !ok {
		return false
	}

	return cv.GetId() == m.ExpectedId && cv.GetVersion() == m.ExpectedVersion
}

func (m ConnectorVersionMatcher) String() string {
	return fmt.Sprintf("is ConnectorVersion with ID=%s, Version=%d", m.ExpectedId, m.ExpectedVersion)
}
