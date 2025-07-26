package connectors

// AuthOauth2Revocation configures token revocation per RFC7009 (https://datatracker.ietf.org/doc/html/rfc7009)
type AuthOauth2Revocation struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
	FormOverrides  map[string]string `json:"form_overrides,omitempty" yaml:"form_overrides,omitempty"`
}
