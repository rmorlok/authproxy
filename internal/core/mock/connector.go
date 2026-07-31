package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

type Connector struct {
	Id          apid.ID
	Namespace   string
	Version     uint64
	State       database.ConnectorDefinitionVersionState
	Type        string
	Hash        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Labels      map[string]string
	Annotations map[string]string
	Definition  *cschema.Connector
}

func (m *Connector) GetId() apid.ID {
	return m.Id
}

func (m *Connector) GetNamespace() string {
	return m.Namespace
}

func (m *Connector) GetVersion() uint64 {
	return m.Version
}

func (m *Connector) GetState() database.ConnectorDefinitionVersionState {
	return m.State
}

func (m *Connector) GetType() string {
	return m.Type
}

func (m *Connector) GetHash() string {
	return m.Hash
}

func (m *Connector) GetCreatedAt() time.Time {
	return m.CreatedAt
}

func (m *Connector) GetUpdatedAt() time.Time {
	return m.UpdatedAt
}

func (m *Connector) GetLabels() map[string]string {
	return m.Labels
}

func (m *Connector) GetAnnotations() map[string]string {
	return m.Annotations
}

func (m *Connector) GetDefinition() *cschema.Connector {
	return m.Definition
}

func (m *Connector) SetState(_ context.Context, state database.ConnectorDefinitionVersionState) error {
	m.State = state
	return nil
}

var _ iface.Connector = (*Connector)(nil)

type ConnectorVersionMatcher struct {
	ExpectedId      apid.ID
	ExpectedVersion uint64
}

func (m ConnectorVersionMatcher) Matches(x interface{}) bool {
	cv, ok := x.(iface.Connector)
	if !ok {
		return false
	}

	return cv.GetId() == m.ExpectedId && cv.GetVersion() == m.ExpectedVersion
}

func (m ConnectorVersionMatcher) String() string {
	return fmt.Sprintf("is ConnectorVersion with ID=%s, Version=%d", m.ExpectedId, m.ExpectedVersion)
}
