package config

import (
	"fmt"
	"time"
)

type ServicePublic struct {
	PortVal                  uint64         `json:"port" yaml:"port"`
	DomainVal                string         `json:"domain" yaml:"domain"`
	IsHttpsVal               bool           `json:"https" yaml:"https"`
	SessionTimeoutVal        *HumanDuration `json:"session_timeout" yaml:"session_timeout"`
	CookieDomainVal          *string        `json:"cookie_domain" yaml:"cookie_domain"`
	XsrfRequestQueueDepthVal *int           `json:"xsrf_request_queue_depth" yaml:"xsrf_request_queue_depth"`
}

func (s *ServicePublic) Port() uint64 {
	return s.PortVal
}

func (s *ServicePublic) IsHttps() bool {
	return s.IsHttpsVal
}

func (s *ServicePublic) Domain() string {
	return s.DomainVal
}

func (s *ServicePublic) GetBaseUrl() string {
	proto := "http"
	if s.IsHttps() {
		proto = "https"
	}

	domain := "localhost"
	if s.Domain() != "" {
		domain = s.Domain()
	}

	if s.Port() == 80 {
		return fmt.Sprintf("%s://%s", proto, domain)
	} else {
		return fmt.Sprintf("%s://%s:%d", proto, domain, s.Port())
	}
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
	if s.CookieDomainVal != nil {
		return *s.CookieDomainVal
	}

	return s.DomainVal
}

func (s *ServicePublic) XsrfRequestQueueDepth() int {
	if s.XsrfRequestQueueDepthVal == nil {
		return 100
	}

	return *s.XsrfRequestQueueDepthVal
}

var _ Service = (*ServicePublic)(nil)
