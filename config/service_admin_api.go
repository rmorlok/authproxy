package config

import (
	"fmt"
	"time"
)

type ServiceAdminApi struct {
	PortVal    uint64 `json:"port" yaml:"port"`
	DomainVal  string `json:"domain" yaml:"domain"`
	IsHttpsVal bool   `json:"https" yaml:"https"`
}

func (s *ServiceAdminApi) Port() uint64 {
	return s.PortVal
}

func (s *ServiceAdminApi) IsHttps() bool {
	return s.IsHttpsVal
}

func (s *ServiceAdminApi) Domain() string {
	return s.DomainVal
}

func (s *ServiceAdminApi) GetBaseUrl() string {
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

func (s *ServiceAdminApi) SupportsSession() bool {
	return false
}

func (s *ServiceAdminApi) SessionTimeout() time.Duration {
	return 0 * time.Second
}

func (s *ServiceAdminApi) CookieDomain() string {
	return ""
}

func (s *ServiceAdminApi) XsrfRequestQueueDepth() int {
	return 0
}

func (s *ServiceAdminApi) GetId() ServiceId {
	return ServiceIdAdminApi
}

var _ Service = (*ServiceAdminApi)(nil)
