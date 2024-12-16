package config

import (
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/util"
	"os"
)

type C interface {
	// GetRoot gets the root of the configuration; the data loaded from a configuration file
	GetRoot() *Root

	// IsDebugMode tells the system if debug flags have been passed when running this service
	IsDebugMode() bool

	// MustApiHostForService gets the host information for the specified service name
	MustApiHostForService(serviceName ServiceId) *ApiHost

	// MustGetAESKey retrieves an AES key from the config that can be used to symmetrically encrypt data temporarily
	MustGetAESKey(ctx context.Context) []byte

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

func (c *config) MustApiHostForService(serviceName ServiceId) *ApiHost {
	r := c.GetRoot()
	if r == nil {
		panic("root config not present")
	}

	return r.MustApiHostForService(serviceName)
}

func (c *config) IsDebugMode() bool {
	return os.Getenv("AUTHPROXY_DEBUG_MODE") == "true"
}

func (c *config) MustGetAESKey(ctx context.Context) []byte {
	return util.Must(c.GetRoot().SystemAuth.GlobalAESKey.GetData(ctx))
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
