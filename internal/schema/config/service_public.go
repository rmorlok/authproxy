package config

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ServicePublicStaticContentConfig is a configuration to have the public service serve static content in addition
// to its other functions. This can be used to serve the marketplace SPA directly.
type ServicePublicStaticContentConfig struct {
	MountAtPath   string `json:"mount_at" yaml:"mount_at"`
	ServeFromPath string `json:"serve_from" yaml:"serve_from"`
}

type CookieConfig struct {
	DomainVal   *string `json:"domain,omitempty" yaml:"domain,omitempty"`
	SameSiteVal *string `json:"same_site,omitempty" yaml:"same_site,omitempty"`
}

type ServicePublic struct {
	ServiceHttp
	SessionTimeoutVal        *HumanDuration                    `json:"session_timeout" yaml:"session_timeout"`
	XsrfRequestQueueDepthVal *int                              `json:"xsrf_request_queue_depth" yaml:"xsrf_request_queue_depth"`
	EnableMarketplaceApisVal *bool                             `json:"enable_marketplace_apis,omitempty" yaml:"enable_marketplace_apis,omitempty"`
	EnableProxyVal           *bool                             `json:"enable_proxy,omitempty" yaml:"enable_proxy,omitempty"`
	StaticVal                *ServicePublicStaticContentConfig `json:"static,omitempty" yaml:"static,omitempty"`
	CookieVal                *CookieConfig                     `json:"cookie,omitempty" yaml:"cookie,omitempty"`
}

func (s *ServicePublic) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	// Let the rest unmarshall normally
	type RawType ServicePublic
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ServiceHttp = hs

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
		// Assume the marketplace is being served from public service, so same site is ok
		return http.SameSiteStrictMode
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
