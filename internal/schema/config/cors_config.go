package config

import "github.com/gin-contrib/cors"

type CorsConfig struct {
	AllowedOrigins   []string       `json:"allowedOrigins,omitempty" yaml:"allowedOrigins,omitempty"`
	AllowedMethods   []string       `json:"allowedMethods,omitempty" yaml:"allowedMethods,omitempty"`
	AllowedHeaders   []string       `json:"allowedHeaders,omitempty" yaml:"allowedHeaders,omitempty"`
	ExposedHeaders   []string       `json:"exposedHeaders,omitempty" yaml:"exposedHeaders,omitempty"`
	MaxAge           *HumanDuration `json:"maxAge,omitempty" yaml:"maxAge,omitempty"`
	AllowCredentials *bool          `json:"allowCredentials,omitempty" yaml:"allowCredentials,omitempty"`
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
