package config

import (
	"context"
	"fmt"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
	"strconv"
	"time"
)

type ServicePublic struct {
	PortVal                  StringValue    `json:"port" yaml:"port"`
	HealthCheckPortVal       StringValue    `json:"health_check_port,omitempty" yaml:"health_check_port,omitempty"`
	DomainVal                string         `json:"domain" yaml:"domain"`
	IsHttpsVal               bool           `json:"https" yaml:"https"`
	SessionTimeoutVal        *HumanDuration `json:"session_timeout" yaml:"session_timeout"`
	CookieDomainVal          *string        `json:"cookie_domain" yaml:"cookie_domain"`
	XsrfRequestQueueDepthVal *int           `json:"xsrf_request_queue_depth" yaml:"xsrf_request_queue_depth"`
}

func (s *ServicePublic) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	var portVal StringValue = &StringValueDirect{Value: "0"}
	var healthCheckPortVal StringValue = nil

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "port":
			if portVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "health_check_port":
			if healthCheckPortVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType ServicePublic
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.PortVal = portVal
	raw.HealthCheckPortVal = healthCheckPortVal

	return nil
}

func (s *ServicePublic) Port() uint64 {
	portS, err := s.PortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain port from public config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse port '%s' from public config", portS))
	}

	return port
}

func (s *ServicePublic) HealthCheckPort() uint64 {
	if s.HealthCheckPortVal == nil {
		return s.Port()
	}

	portS, err := s.HealthCheckPortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain health check port from public config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse health check port '%s' from public config", portS))
	}

	return port
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
