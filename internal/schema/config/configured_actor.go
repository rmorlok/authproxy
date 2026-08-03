package config

import (
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type ConfiguredActor struct {
	ExternalId  string               `json:"externalId" yaml:"externalId"`
	Key         *Key                 `json:"key" yaml:"key"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
}
