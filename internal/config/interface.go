package config

import (
	"log/slog"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type C interface {
	// Validate checks that the configuration is valid
	Validate() error

	// GetRoot gets the root of the configuration; the data loaded from a configuration file
	GetRoot() *sconfig.Root

	// IsDebugMode tells the system if debug flags have been passed when running this service
	IsDebugMode() bool

	// MustGetService gets the service information for the specified service name
	MustGetService(serviceName sconfig.ServiceId) sconfig.Service

	// GetFallbackConnectorLogo gets a logo to use if not specified for a connector configuration
	GetFallbackConnectorLogo() string

	// GetErrorPageUrl gets a URL to an error page for the specified error. If explicitly set in Root.ErrorPages, it
	// uses that value. If not, falls back to defaults
	GetErrorPageUrl(sconfig.ErrorPage) string

	// GetRootLogger returns the root logger instance configured for the application. This will always
	// return a logger, defaulting to a none logger if nothing is configured.
	GetRootLogger() *slog.Logger

	// GetGlobalKey returns the global key for the application. This is used for symmetric encryption of data in things
	// like cursors, JWTs, etc.
	GetGlobalKey() sconfig.KeyDataType
}
