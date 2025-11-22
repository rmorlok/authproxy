package core

import (
	"errors"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
)

type connectorVersionBuilder struct {
	s              *service
	c              *config.Connector
	configSetters  []func(c *config.Connector)
	versionSetters []func(v *ConnectorVersion)
}

func newConnectorVersionBuilder(s *service) *connectorVersionBuilder {
	return &connectorVersionBuilder{
		s: s,
	}
}

func (b *connectorVersionBuilder) WithConfig(c *config.Connector) *connectorVersionBuilder {
	b.c = c

	b.versionSetters = append([]func(v *ConnectorVersion){
		func(v *ConnectorVersion) {
			v.Version = c.Version
			v.Type = c.Type
			v.ID = c.Id
			v.NamespacePath = c.GetNamespacePath()
		},
	}, b.versionSetters...)

	return b
}

func (b *connectorVersionBuilder) WithId(id uuid.UUID) *connectorVersionBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *ConnectorVersion) {
			v.ID = id
		},
	)

	b.configSetters = append(b.configSetters,
		func(c *config.Connector) {
			c.Id = id
		},
	)
	return b
}

func (b *connectorVersionBuilder) WithType(t string) *connectorVersionBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *ConnectorVersion) {
			v.Type = t
		},
	)

	b.configSetters = append(b.configSetters,
		func(c *config.Connector) {
			c.Type = t
		},
	)
	return b
}

func (b *connectorVersionBuilder) WithState(state database.ConnectorVersionState) *connectorVersionBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *ConnectorVersion) {
			v.State = state
		},
	)

	b.configSetters = append(b.configSetters,
		func(c *config.Connector) {
			c.State = string(state)
		},
	)
	return b
}

func (b *connectorVersionBuilder) WithVersion(ver uint64) *connectorVersionBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *ConnectorVersion) {
			v.Version = ver
		},
	)

	b.configSetters = append(b.configSetters,
		func(c *config.Connector) {
			c.Version = uint64(ver)
		},
	)
	return b
}

var errNilConnector = errors.New("nil connector")

func (b *connectorVersionBuilder) Build() (*ConnectorVersion, error) {
	if b.c == nil {
		return nil, errNilConnector
	}

	cv := ConnectorVersion{
		s: b.s,
	}

	for _, setter := range b.configSetters {
		setter(b.c)
	}

	for _, setter := range b.versionSetters {
		setter(&cv)
	}

	if err := cv.setDefinition(b.c); err != nil {
		return nil, err
	}

	return &cv, nil
}
