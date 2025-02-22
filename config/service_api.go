package config

import (
	"fmt"
	"time"
)

type ServiceApi struct {
	PortVal    uint64 `json:"port" yaml:"port"`
	DomainVal  string `json:"domain" yaml:"domain"`
	IsHttpsVal bool   `json:"https" yaml:"https"`
}

func (s *ServiceApi) Port() uint64 {
	return s.PortVal
}

func (s *ServiceApi) IsHttps() bool {
	return s.IsHttpsVal
}

func (s *ServiceApi) Domain() string {
	return s.DomainVal
}

func (s *ServiceApi) GetBaseUrl() string {
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

func (s *ServiceApi) SupportsSession() bool {
	return false
}

func (s *ServiceApi) SessionTimeout() time.Duration {
	return 0 * time.Second
}

func (s *ServiceApi) CookieDomain() string {
	return ""
}

func (s *ServiceApi) XsrfRequestQueueDepth() int {
	return 0
}

func (s *ServiceApi) GetId() ServiceId {
	return ServiceIdApi
}

var _ Service = (*ServiceApi)(nil)
