package config

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ServicePublicStaticContentConfig is a configuration to have the public service serve static content in addition
// to its other functions. This can be used to serve the marketplace SPA directly.
//
// When ServeFromPath is empty, the service serves the UI from its compiled-in
// embedded filesystem (built by `vite build` and bundled via //go:embed). Set
// ServeFromPath to override with an on-disk build for local iteration or
// custom branding.
type ServicePublicStaticContentConfig struct {
	MountAtPath   string `json:"mountAt" yaml:"mountAt"`
	ServeFromPath string `json:"serveFrom" yaml:"serveFrom"`
}

// IsEmbedded reports whether the static handler should serve from the service's
// compiled-in UI assets rather than an on-disk directory.
func (c *ServicePublicStaticContentConfig) IsEmbedded() bool {
	return c != nil && c.ServeFromPath == ""
}

type CookieConfig struct {
	DomainVal   *string `json:"domain,omitempty" yaml:"domain,omitempty"`
	SameSiteVal *string `json:"sameSite,omitempty" yaml:"sameSite,omitempty"`
}

type ServicePublic struct {
	ServiceHttp
	SessionTimeoutVal        *HumanDuration                    `json:"sessionTimeout" yaml:"sessionTimeout"`
	XsrfRequestQueueDepthVal *int                              `json:"xsrfRequestQueueDepth" yaml:"xsrfRequestQueueDepth"`
	EnableMarketplaceApisVal *bool                             `json:"enableMarketplaceApis,omitempty" yaml:"enableMarketplaceApis,omitempty"`
	EnableProxyVal           *bool                             `json:"enableProxy,omitempty" yaml:"enableProxy,omitempty"`
	StaticVal                *ServicePublicStaticContentConfig `json:"static,omitempty" yaml:"static,omitempty"`
	CookieVal                *CookieConfig                     `json:"cookie,omitempty" yaml:"cookie,omitempty"`
}

func (s *ServicePublic) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}
	publicFields := []string{
		"sessionTimeout",
		"xsrfRequestQueueDepth",
		"enableMarketplaceApis",
		"enableProxy",
		"static",
		"cookie",
	}
	if err := validateYAMLMappingFields(value, append(httpServiceYAMLFields, publicFields...)...); err != nil {
		return err
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	type rawServicePublic struct {
		SessionTimeoutVal        *HumanDuration                    `yaml:"sessionTimeout"`
		XsrfRequestQueueDepthVal *int                              `yaml:"xsrfRequestQueueDepth"`
		EnableMarketplaceApisVal *bool                             `yaml:"enableMarketplaceApis,omitempty"`
		EnableProxyVal           *bool                             `yaml:"enableProxy,omitempty"`
		StaticVal                *ServicePublicStaticContentConfig `yaml:"static,omitempty"`
		CookieVal                *CookieConfig                     `yaml:"cookie,omitempty"`
	}
	raw := &rawServicePublic{}
	if err := util.DecodeYAMLNodeStrict(yamlMappingWithFields(value, publicFields...), raw); err != nil {
		return err
	}

	s.ServiceHttp = hs
	s.SessionTimeoutVal = raw.SessionTimeoutVal
	s.XsrfRequestQueueDepthVal = raw.XsrfRequestQueueDepthVal
	s.EnableMarketplaceApisVal = raw.EnableMarketplaceApisVal
	s.EnableProxyVal = raw.EnableProxyVal
	s.StaticVal = raw.StaticVal
	s.CookieVal = raw.CookieVal

	return nil
}

func (s *ServicePublic) SupportsSession() bool {
	return true
}

func (s *ServicePublic) GetId() ServiceId {
	return ServiceIdPublic
}

func (s *ServicePublic) SessionTimeout() time.Duration {
	if s.SessionTimeoutVal == nil {
		return 1 * time.Hour
	}

	return s.SessionTimeoutVal.Duration
}

func (s *ServicePublic) CookieDomain() string {
	if s.CookieVal != nil && s.CookieVal.DomainVal != nil {
		return *s.CookieVal.DomainVal
	}

	return s.DomainVal
}

func (s *ServicePublic) CookieSameSite() http.SameSite {
	if s.CookieVal != nil && s.CookieVal.SameSiteVal != nil {
		switch strings.ToLower(*s.CookieVal.SameSiteVal) {
		case "none":
			return http.SameSiteNoneMode
		case "lax":
			return http.SameSiteLaxMode
		case "strict":
			return http.SameSiteStrictMode
		default:
			return http.SameSiteDefaultMode
		}
	}

	if s.StaticVal != nil {
		// Marketplace shares the public origin, but OAuth providers redirect the
		// browser back to /oauth2/callback as a cross-site top-level navigation.
		// Strict drops the session cookie on that hop and dead-ends the flow at
		// the unauth redirect; Lax keeps CSRF protection on subresource POSTs
		// while allowing the callback to authenticate.
		return http.SameSiteLaxMode
	}

	return http.SameSiteNoneMode
}

func (s *ServicePublic) XsrfRequestQueueDepth() int {
	if s.XsrfRequestQueueDepthVal == nil {
		return 100
	}

	return *s.XsrfRequestQueueDepthVal
}

// EnableMarketplaceApis determines if the APIs to support the marketplace are exposed on the public API to make
// them available via session. Defaults to true if not set. Disable this feature if the host application is wrapping
// the API service directly with its own custom marketplace app.
func (s *ServicePublic) EnableMarketplaceApis() bool {
	if s == nil || s.EnableMarketplaceApisVal == nil {
		return true
	}

	return *s.EnableMarketplaceApisVal
}

// EnableProxy determines if proxying to 3rd parties is enabled on the public service. Defaults to false if unspecified.
// Enabling the 3rd party proxy on public can allow custom logic in the marketplace where the client makes calls
// directly to the 3rd party. This increases the surface area for security risks, however.
func (s *ServicePublic) EnableProxy() bool {
	if s == nil || s.EnableProxyVal == nil {
		return false
	}

	return *s.EnableProxyVal
}

var _ HttpServiceWithSession = (*ServicePublic)(nil)
