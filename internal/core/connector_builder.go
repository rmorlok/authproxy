package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

type connectorBuilder struct {
	s                 *service
	definition        *connectors.ConnectorDefinition
	generationSetters []func(v *Connector)
}

func newConnectorBuilder(s *service) *connectorBuilder {
	return &connectorBuilder{
		s: s,
	}
}

func (b *connectorBuilder) WithDefinition(definition *connectors.ConnectorDefinition) *connectorBuilder {
	b.definition = definition
	return b
}

func (b *connectorBuilder) WithId(id apid.ID) *connectorBuilder {
	b.generationSetters = append(b.generationSetters,
		func(v *Connector) {
			v.Id = id
		},
	)

	return b
}

func (b *connectorBuilder) WithState(state database.ConnectorGenerationState) *connectorBuilder {
	b.generationSetters = append(b.generationSetters,
		func(v *Connector) {
			v.State = state
		},
	)

	return b
}

func (b *connectorBuilder) WithGeneration(ver uint64) *connectorBuilder {
	b.generationSetters = append(b.generationSetters,
		func(v *Connector) {
			v.Generation = ver
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

	for _, setter := range b.generationSetters {
		setter(&c)
	}

	if err := c.setDefinition(b.definition); err != nil {
		return nil, err
	}

	return &c, nil
}
