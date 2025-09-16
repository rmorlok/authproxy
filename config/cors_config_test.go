package config

import (
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/stretchr/testify/assert"
)

func TestToGinCorsConfig(t *testing.T) {
	trueVal := true
	falseVal := false
	defaults := &cors.Config{
		AllowOrigins:  []string{"https://example.com/", "http://example.org"},
		AllowMethods:  []string{"GET", "POST"},
		AllowHeaders:  []string{"Content-Type", "Authorization"},
		ExposeHeaders: []string{"X-Custom-Header"},
		MaxAge:        12 * time.Hour,
	}

	tests := []struct {
		name     string
		config   *CorsConfig
		defaults *cors.Config
		expected *cors.Config
	}{
		{
			name:     "nil config and defaults",
			config:   nil,
			defaults: nil,
			expected: nil,
		},
		{
			name:   "nil config, with defaults",
			config: nil,
			defaults: &cors.Config{
				AllowOrigins:  []string{"http://example.com"},
				AllowMethods:  []string{"GET"},
				AllowHeaders:  []string{"Content-Type"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        24 * time.Hour,
			},
			expected: &cors.Config{
				AllowOrigins:  []string{"http://example.com"},
				AllowMethods:  []string{"GET"},
				AllowHeaders:  []string{"Content-Type"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        24 * time.Hour,
			},
		},
		{
			name: "trimmed origins",
			config: &CorsConfig{
				AllowedOrigins: []string{"https://example.com/", "http://example.org/"},
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:  []string{"https://example.com", "http://example.org"},
				AllowMethods:  []string{"GET", "POST"},
				AllowHeaders:  []string{"Content-Type", "Authorization"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        12 * time.Hour,
			},
		},
		{
			name: "overridden methods",
			config: &CorsConfig{
				AllowedMethods: []string{"PUT", "DELETE"},
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:  []string{"https://example.com/", "http://example.org"},
				AllowMethods:  []string{"PUT", "DELETE"},
				AllowHeaders:  []string{"Content-Type", "Authorization"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        12 * time.Hour,
			},
		},
		{
			name: "allow credentials enabled",
			config: &CorsConfig{
				AllowCredentials: &trueVal,
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:     []string{"https://example.com/", "http://example.org"},
				AllowMethods:     []string{"GET", "POST"},
				AllowHeaders:     []string{"Content-Type", "Authorization"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				MaxAge:           12 * time.Hour,
				AllowCredentials: true,
			},
		},
		{
			name: "allow credentials disabled",
			config: &CorsConfig{
				AllowCredentials: &falseVal,
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:     []string{"https://example.com/", "http://example.org"},
				AllowMethods:     []string{"GET", "POST"},
				AllowHeaders:     []string{"Content-Type", "Authorization"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				MaxAge:           12 * time.Hour,
				AllowCredentials: false,
			},
		},
		{
			name: "overridden headers",
			config: &CorsConfig{
				AllowedHeaders: []string{"X-New-Header"},
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:  []string{"https://example.com/", "http://example.org"},
				AllowMethods:  []string{"GET", "POST"},
				AllowHeaders:  []string{"X-New-Header"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        12 * time.Hour,
			},
		},
		{
			name: "custom max age",
			config: &CorsConfig{
				MaxAge: &HumanDuration{Duration: 1 * time.Hour},
			},
			defaults: defaults,
			expected: &cors.Config{
				AllowOrigins:  []string{"https://example.com/", "http://example.org"},
				AllowMethods:  []string{"GET", "POST"},
				AllowHeaders:  []string{"Content-Type", "Authorization"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        1 * time.Hour,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ToGinCorsConfig(tt.defaults)
			assert.Equal(t, tt.expected, result)
		})
	}
}
