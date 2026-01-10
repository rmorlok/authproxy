package config

import (
	"log/slog"
	"os"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type config struct {
	root *sconfig.Root
}

func (c *config) Validate() error {
	return c.root.Validate()
}

func (c *config) GetRoot() *sconfig.Root {
	if c == nil {
		return nil
	}

	return c.root
}

func (c *config) MustGetService(serviceName sconfig.ServiceId) sconfig.Service {
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

func (c *config) GetErrorPageUrl(ep sconfig.ErrorPage) string {
	return c.root.ErrorPages.UrlForError(ep, c.root.Public.GetBaseUrl())
}

func (c *config) GetGlobalKey() sconfig.KeyDataType {
	if c == nil {
		return nil
	}

	if c.root == nil {
		return nil
	}

	return c.root.SystemAuth.GlobalAESKey
}
