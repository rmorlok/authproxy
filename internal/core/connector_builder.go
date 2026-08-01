package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type connectorBuilder struct {
	s              *service
	c              *config.Connector
	configSetters  []func(c *config.Connector)
	versionSetters []func(v *Connector)
}

func newConnectorBuilder(s *service) *connectorBuilder {
	return &connectorBuilder{
		s: s,
	}
}

func (b *connectorBuilder) WithConfig(c *config.Connector) *connectorBuilder {
	b.c = c

	b.versionSetters = append([]func(v *Connector){
		func(v *Connector) {
			v.Version = c.Version
			v.Id = c.Id
			v.Namespace = c.GetNamespace()
			v.Labels = c.Labels
		},
	}, b.versionSetters...)

	return b
}

func (b *connectorBuilder) WithId(id apid.ID) *connectorBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *Connector) {
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

func (b *connectorBuilder) WithState(state database.ConnectorDefinitionVersionState) *connectorBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *Connector) {
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

func (b *connectorBuilder) WithVersion(ver uint64) *connectorBuilder {
	b.versionSetters = append(b.versionSetters,
		func(v *Connector) {
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

func (b *connectorBuilder) Build() (*Connector, error) {
	if b.c == nil {
		return nil, errNilConnector
	}

	c := Connector{
		s: b.s,
	}

	for _, setter := range b.configSetters {
		setter(b.c)
	}

	for _, setter := range b.versionSetters {
		setter(&c)
	}

	if err := c.setDefinition(b.c); err != nil {
		return nil, err
	}

	return &c, nil
}
