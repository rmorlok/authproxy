package config

import (
	"context"
	"fmt"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
	"strconv"
	"time"
)

type ServiceMarketplace struct {
	PortVal            StringValue `json:"port,omitempty" yaml:"port"`
	HealthCheckPortVal StringValue `json:"health_check_port,omitempty" yaml:"health_check_port,omitempty"`
	DomainVal          string      `json:"domain,omitempty" yaml:"domain,omitempty"`
	IsHttpsVal         bool        `json:"https,omitempty" yaml:"https,omitempty"`
}

func (s *ServiceMarketplace) UnmarshalYAML(value *yaml.Node) error {
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
	type RawType ServiceMarketplace
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.PortVal = portVal
	raw.HealthCheckPortVal = healthCheckPortVal

	return nil
}

func (s *ServiceMarketplace) Port() uint64 {
	portS, err := s.PortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain port from marketplace config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse port '%s' from marketplace config", portS))
	}

	return port
}

func (s *ServiceMarketplace) HealthCheckPort() uint64 {
	if s.HealthCheckPortVal == nil {
		return s.Port()
	}

	portS, err := s.HealthCheckPortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain health check port from marketplace config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse health check port '%s' from marketplace config", portS))
	}

	return port
}

func (s *ServiceMarketplace) IsHttps() bool {
	return s.IsHttpsVal
}

func (s *ServiceMarketplace) Domain() string {
	return s.DomainVal
}

func (s *ServiceMarketplace) GetBaseUrl() string {
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

func (s *ServiceMarketplace) SupportsSession() bool {
	return true
}

func (s *ServiceMarketplace) GetId() ServiceId {
	return ServiceIdMarketplace
}

func (s *ServiceMarketplace) SessionTimeout() time.Duration {
	return 0
}

func (s *ServiceMarketplace) CookieDomain() string {
	return ""
}

func (s *ServiceMarketplace) XsrfRequestQueueDepth() int {
	return 0
}

var _ Service = (*ServiceMarketplace)(nil)
