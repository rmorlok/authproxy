package config

import (
	"os"
)

type C interface {
	// GetRoot gets the root of the configuration; the data loaded from a configuration file
	GetRoot() *Root

	// IsDebugMode tells the system if debug flags have been passed when running this service
	IsDebugMode() bool

	// MustGetService gets the service information for the specified service name
	MustGetService(serviceName ServiceId) Service

	// GetFallbackConnectorLogo gets a logo to use if not specified for a connector configuration
	GetFallbackConnectorLogo() string
}

type config struct {
	root *Root
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
