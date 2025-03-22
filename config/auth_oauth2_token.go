package config

type AuthOauth2Token struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
	FormOverrides  map[string]string `json:"form_overrides,omitempty" yaml:"form_overrides,omitempty"`
}
