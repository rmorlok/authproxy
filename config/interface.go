package config

import (
	"log/slog"
	"os"
)

type C interface {
	// Validate checks that the configuration is valid
	Validate() error

	// GetRoot gets the root of the configuration; the data loaded from a configuration file
	GetRoot() *Root

	// IsDebugMode tells the system if debug flags have been passed when running this service
	IsDebugMode() bool

	// MustGetService gets the service information for the specified service name
	MustGetService(serviceName ServiceId) Service

	// GetFallbackConnectorLogo gets a logo to use if not specified for a connector configuration
	GetFallbackConnectorLogo() string

	// GetErrorPageUrl gets a URL to an error page for the specified error. If explicitly set in Root.ErrorPages, it
	// uses that value. If not, falls back to defaults
	GetErrorPageUrl(ErrorPage) string

	// GetRootLogger returns the root logger instance configured for the application. This will always
	// return a logger, defaulting to a none logger if nothing is configured.
	GetRootLogger() *slog.Logger

	// GetGlobalKey returns the global key for the application. This is used for symmetric encryption of data in things
	// like cursors, JWTs, etc.
	GetGlobalKey() KeyData
}

type config struct {
	root *Root
}

func (c *config) Validate() error {
	return c.root.Validate()
}

func (c *config) GetRoot() *Root {
	if c == nil {
		return nil
	}

	return c.root
}

func (c *config) MustGetService(serviceName ServiceId) Service {
	r := c.GetRoot()
	if r == nil {
		panic("root config not present")
	}

	return r.MustGetService(serviceName)
}

func (c *config) IsDebugMode() bool {
	return os.Getenv("AUTHPROXY_DEBUG_MODE") == "true"
}

func (c *config) GetFallbackConnectorLogo() string {
	return "https://upload.wikimedia.org/wikipedia/commons/a/ac/No_image_available.svg"
}

func (c *config) GetRootLogger() *slog.Logger {
	return c.root.GetRootLogger()
}

func (c *config) GetErrorPageUrl(ep ErrorPage) string {
	return c.root.ErrorPages.urlForError(ep, c.root.Public.GetBaseUrl())
}

func (c *config) GetGlobalKey() KeyData {
	if c == nil {
		return nil
	}

	if c.root == nil {
		return nil
	}

	return c.root.SystemAuth.GlobalAESKey
}

func LoadConfig(path string) (C, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	root, err := UnmarshallYamlRoot(content)
	if err != nil {
		return nil, err
	}

	return &config{root: root}, nil
}

func FromRoot(root *Root) C {
	return &config{root: root}
}
