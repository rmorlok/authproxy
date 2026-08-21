package config

import (
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type ConfiguredActor struct {
	ExternalId  string               `json:"externalId" yaml:"externalId"`
	Namespace   string               `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Key         *Key                 `json:"key" yaml:"key"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
}

func (a *ConfiguredActor) GetNamespace() string {
	if a == nil || a.Namespace == "" {
		return RootNamespace
	}
	return a.Namespace
}
