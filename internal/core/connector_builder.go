package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

type connectorBuilder struct {
	s              *service
	c              *config.Connector
	definition     *connectors.ConnectorDefinition
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
	b.definition = &c.Spec.Definition

	b.versionSetters = append([]func(v *Connector){
		func(v *Connector) {
			v.Version = c.Metadata.Generation
			v.Id = c.GetId()
			v.Namespace = c.GetNamespace()
			v.Name = c.Metadata.Name
			v.Labels = c.Metadata.Labels
			v.Annotations = c.Metadata.Annotations
		},
	}, b.versionSetters...)

	return b
}

func (b *connectorBuilder) WithDefinition(definition *connectors.ConnectorDefinition) *connectorBuilder {
	b.definition = definition
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
			c.SetId(id)
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
			c.Spec.Release.DesiredState = connectors.ConnectorReleaseState(state)
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
			c.Metadata.Generation = ver
		},
	)
	return b
}

var errNilConnector = errors.New("nil connector")

func (b *connectorBuilder) Build() (*Connector, error) {
	if b.definition == nil {
		return nil, errNilConnector
	}

	c := Connector{
		s: b.s,
	}

	if b.c != nil {
		for _, setter := range b.configSetters {
			setter(b.c)
		}
	}

	for _, setter := range b.versionSetters {
		setter(&c)
	}

	if err := c.setDefinition(b.definition); err != nil {
		return nil, err
	}

	return &c, nil
}
