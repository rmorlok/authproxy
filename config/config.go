package config

import "os"

type Config interface {
	GetRoot() *Root
	IsDebugMode() bool
}

type config struct {
	root *Root
}

func (c *config) GetRoot() *Root {
	return c.root
}

func (c *config) IsDebugMode() bool {
	return os.Getenv("AUTHPROXY_DEBUG_MODE") == "true"
}

func LoadConfig(path string) (Config, error) {
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
