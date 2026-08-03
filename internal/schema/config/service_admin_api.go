package config

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type ServiceAdminApi struct {
	ServiceHttp
	Ui                       *ServiceAdminUi                   `json:"ui" yaml:"ui"`
	SessionTimeoutVal        *HumanDuration                    `json:"sessionTimeout" yaml:"sessionTimeout"`
	XsrfRequestQueueDepthVal *int                              `json:"xsrfRequestQueueDepth" yaml:"xsrfRequestQueueDepth"`
	StaticVal                *ServicePublicStaticContentConfig `json:"static,omitempty" yaml:"static,omitempty"`
	CookieVal                *CookieConfig                     `json:"cookie,omitempty" yaml:"cookie,omitempty"`
}

func (s *ServiceAdminApi) SessionTimeout() time.Duration {
	if !s.SupportsSession() {
		panic("admin api not configured to support session")
	}

	if s.SessionTimeoutVal == nil {
		return 1 * time.Hour
	}

	return s.SessionTimeoutVal.Duration
}

func (s *ServiceAdminApi) CookieDomain() string {
	if !s.SupportsSession() {
		panic("admin api not configured to support session")
	}

	if s.CookieVal != nil && s.CookieVal.DomainVal != nil {
		return *s.CookieVal.DomainVal
	}

	return s.DomainVal
}

func (s *ServiceAdminApi) CookieSameSite() http.SameSite {
	if !s.SupportsSession() {
		panic("admin api not configured to support session")
	}

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

func (s *ServiceAdminApi) XsrfRequestQueueDepth() int {
	if !s.SupportsSession() {
		panic("admin api not configured to support session")
	}

	if s.XsrfRequestQueueDepthVal == nil {
		return 100
	}

	return *s.XsrfRequestQueueDepthVal
}

func (s *ServiceAdminApi) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}
	adminFields := []string{
		"ui",
		"sessionTimeout",
		"xsrfRequestQueueDepth",
		"static",
		"cookie",
	}
	if err := validateYAMLMappingFields(value, append(httpServiceYAMLFields, adminFields...)...); err != nil {
		return err
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	type rawServiceAdminApi struct {
		Ui                       *ServiceAdminUi                   `yaml:"ui"`
		SessionTimeoutVal        *HumanDuration                    `yaml:"sessionTimeout"`
		XsrfRequestQueueDepthVal *int                              `yaml:"xsrfRequestQueueDepth"`
		StaticVal                *ServicePublicStaticContentConfig `yaml:"static,omitempty"`
		CookieVal                *CookieConfig                     `yaml:"cookie,omitempty"`
	}
	raw := &rawServiceAdminApi{}
	if err := util.DecodeYAMLNodeStrict(yamlMappingWithFields(value, adminFields...), raw); err != nil {
		return err
	}

	s.ServiceHttp = hs
	s.Ui = raw.Ui
	s.SessionTimeoutVal = raw.SessionTimeoutVal
	s.XsrfRequestQueueDepthVal = raw.XsrfRequestQueueDepthVal
	s.StaticVal = raw.StaticVal
	s.CookieVal = raw.CookieVal

	return nil
}

func (s *ServiceAdminApi) SupportsUi() bool {
	if s == nil {
		return false
	}

	if s.Ui == nil {
		return false
	}

	return s.Ui.Enabled
}

func (s *ServiceAdminApi) UiBaseUrl() string {
	if !s.SupportsUi() {
		return ""
	}

	if !s.Ui.BaseUrl.HasValue(context.Background()) {
		return ""
	}

	return util.Must(s.Ui.BaseUrl.GetValue(context.Background()))
}

func (s *ServiceAdminApi) SupportsSession() bool {
	return s.SupportsUi()
}

func (s *ServiceAdminApi) GetId() ServiceId {
	return ServiceIdAdminApi
}

type ServiceAdminUi struct {
	Enabled bool         `json:"enabled" yaml:"enabled"`
	BaseUrl *StringValue `json:"baseUrl" yaml:"baseUrl"`

	// InitiateSessionUrl is the URL that will be redirected to in order to establish a session for an actor. This
	// happens if the admin portal is accessed without coming from a pre-authorized context. This URL should
	// take a `returnToUrl` query parameter where the actor should be redirected to following successful authentication.
	// When redirecting to `returnToUrl`, the host application should append an `authToken` query param with a signed
	// JWT for authenticating the user. This JWT should use a nonce and expiration to protect against session
	// hijacking
	InitiateSessionUrl *StringValue `json:"initiateSessionUrl" yaml:"initiateSessionUrl"`
}

func (s *ServiceAdminUi) GetInitiateSessionUrl(returnTo string) string {
	raw := ""
	if s.InitiateSessionUrl != nil {
		raw, _ = s.InitiateSessionUrl.GetValue(context.Background())
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	q := u.Query()
	q.Set("returnToUrl", returnTo)
	u.RawQuery = q.Encode()

	return u.String()
}

var _ HttpService = (*ServiceAdminApi)(nil)
var _ HttpServiceWithSession = (*ServiceAdminApi)(nil)
