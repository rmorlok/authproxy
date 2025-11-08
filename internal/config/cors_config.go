package config

import "github.com/gin-contrib/cors"

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
		if defaults == nil {
			return nil
		}

		updated := *defaults

		if updated.AllowOrigins != nil {
			// The gin-contrib/cors library does not allow trailing slashes in the allowed origins
			processedOrigins := make([]string, len(updated.AllowOrigins))
			for i, origin := range updated.AllowOrigins {
				if len(origin) > 0 && origin[len(origin)-1] == '/' {
					processedOrigins[i] = origin[:len(origin)-1]
				} else {
					processedOrigins[i] = origin
				}
			}
			updated.AllowOrigins = processedOrigins
		}

		return &updated
	}

	result := cors.Config{}
	if defaults != nil {
		result = *defaults
	}

	if c.AllowedOrigins != nil {

		// The gin-contrib/cors library does not allow trailing slashes in the allowed origins
		processedOrigins := make([]string, len(c.AllowedOrigins))
		for i, origin := range c.AllowedOrigins {
			if len(origin) > 0 && origin[len(origin)-1] == '/' {
				processedOrigins[i] = origin[:len(origin)-1]
			} else {
				processedOrigins[i] = origin
			}
		}

		result.AllowOrigins = processedOrigins
	}

	if c.AllowedMethods != nil {
		result.AllowMethods = c.AllowedMethods
	}

	if c.AllowedHeaders != nil {
		result.AllowHeaders = c.AllowedHeaders
	}

	if c.ExposedHeaders != nil {
		result.ExposeHeaders = c.ExposedHeaders
	}

	if c.MaxAge != nil {
		result.MaxAge = c.MaxAge.Duration
	}

	if c.AllowCredentials != nil {
		result.AllowCredentials = *c.AllowCredentials
	}

	return &result
}
