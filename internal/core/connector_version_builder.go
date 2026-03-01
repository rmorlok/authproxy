package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
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
			v.Id = c.Id
			v.Namespace = c.GetNamespace()
			v.Labels = c.Labels
		},
	}, b.versionSetters...)

	return b
}

func (b *connectorVersionBuilder) WithId(id apid.ID) *connectorVersionBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *ConnectorVersion) {
			v.Id = id
		},
	)

	b.configSetters = append(b.configSetters,
		func(c *config.Connector) {
			c.Id = id
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
