package core

import (
	"errors"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
)

type versionBuilder struct {
	s              *service
	c              *config.Connector
	configSetters  []func(c *config.Connector)
	versionSetters []func(v *ConnectorVersion)
}

func newVersionBuilder(s *service) *versionBuilder {
	return &versionBuilder{
		s: s,
	}
}

func (b *versionBuilder) WithConfig(c *config.Connector) *versionBuilder {
	b.c = c

	b.versionSetters = append([]func(v *ConnectorVersion){
		func(v *ConnectorVersion) {
			v.Version = c.Version
			v.Type = c.Type
			v.ID = c.Id
		},
	}, b.versionSetters...)

	return b
}

func (b *versionBuilder) WithId(id uuid.UUID) *versionBuilder {
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

func (b *versionBuilder) WithType(t string) *versionBuilder {
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

func (b *versionBuilder) WithState(state database.ConnectorVersionState) *versionBuilder {
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

func (b *versionBuilder) WithVersion(ver uint64) *versionBuilder {
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

func (b *versionBuilder) Build() (*ConnectorVersion, error) {
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
