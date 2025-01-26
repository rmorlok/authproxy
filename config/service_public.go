package config

import "fmt"

type ServicePublic struct {
	PortVal    uint64 `json:"port" yaml:"port"`
	DomainVal  string `json:"domain" yaml:"domain"`
	IsHttpsVal bool   `json:"https" yaml:"https"`
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

var _ Service = (*ServicePublic)(nil)
