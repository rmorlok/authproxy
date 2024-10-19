package config

import "os"

type C interface {
	GetRoot() *Root
	IsDebugMode() bool
	MustApiHostForService(serviceName ServiceId) *ApiHost
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

func ConfigFromRoot(root *Root) C {
	return &config{root: root}
}
