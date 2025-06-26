package config

import "github.com/gin-gonic/contrib/cors"

type CorsConfig struct {
	AllowedOrigins   []string       `json:"allowed_origins,omitempty" yaml:"allowed_origins,omitempty"`
	AllowedMethods   []string       `json:"allowed_methods,omitempty" yaml:"allowed_methods,omitempty"`
	AllowedHeaders   []string       `json:"allowed_headers,omitempty" yaml:"allowed_headers,omitempty"`
	ExposedHeaders   []string       `json:"exposed_headers,omitempty" yaml:"exposed_headers,omitempty"`
	MaxAge           *HumanDuration `json:"max_age,omitempty" yaml:"max_age,omitempty"`
	AllowCredentials *bool          `json:"allow_credentials,omitempty" yaml:"allow_credentials,omitempty"`
}

func (c *CorsConfig) ToGinCorsConfig(defaults *cors.Config) *cors.Config {
	if c == nil {
		return defaults
	}

	result := cors.Config{}
	if defaults != nil {
		result = *defaults
	}

	if c.AllowedOrigins != nil {
		result.AllowedOrigins = c.AllowedOrigins
	}

	if c.AllowedMethods != nil {
		result.AllowedMethods = c.AllowedMethods
	}

	if c.AllowedHeaders != nil {
		result.AllowedHeaders = c.AllowedHeaders
	}

	if c.ExposedHeaders != nil {
		result.ExposedHeaders = c.ExposedHeaders
	}

	if c.MaxAge != nil {
		result.MaxAge = c.MaxAge.Duration
	}

	if c.AllowCredentials != nil {
		result.AllowCredentials = *c.AllowCredentials
	}

	return &result
}
