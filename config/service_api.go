package config

import (
	"fmt"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/yaml.v3"
	"strconv"
	"time"
)

type ServiceApi struct {
	PortVal            StringValue `json:"port" yaml:"port"`
	HealthCheckPortVal StringValue `json:"health_check_port,omitempty" yaml:"health_check_port,omitempty"`
	DomainVal          string      `json:"domain" yaml:"domain"`
	IsHttpsVal         bool        `json:"https" yaml:"https"`
}

func (s *ServiceApi) UnmarshalYAML(value *yaml.Node) error {
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
			if portVal, err = stringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "health_check_port":
			if healthCheckPortVal, err = stringValueUnmarshalYAML(valueNode); err != nil {
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
	type RawType ServiceApi
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.PortVal = portVal
	raw.HealthCheckPortVal = healthCheckPortVal

	return nil
}

func (s *ServiceApi) Port() uint64 {
	portS, err := s.PortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain port from admin api config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse port '%s' from admin api config", portS))
	}

	return port
}

func (s *ServiceApi) HealthCheckPort() uint64 {
	if s.HealthCheckPortVal == nil {
		return s.Port()
	}

	portS, err := s.HealthCheckPortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain health check port from admin api config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse health check port '%s' from admin api config", portS))
	}

	return port
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
