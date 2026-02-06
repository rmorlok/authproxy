package connectors

type AuthOauth2Authorization struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
}
