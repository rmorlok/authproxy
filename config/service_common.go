package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
	"net/http"
	"strconv"
)

type ServiceCommon struct {
	HealthCheckPortVal StringValue `json:"health_check_port,omitempty" yaml:"health_check_port,omitempty"`
}

func commonServiceUnmarshalYAML(value *yaml.Node) (ServiceCommon, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return ServiceCommon{}, fmt.Errorf("commonService expected a mapping node, got %s", KindToString(value.Kind))
	}

	var healthCheckPortVal StringValue = &StringValueDirect{Value: "0"}

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "health_check_port":
			if healthCheckPortVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return ServiceCommon{}, err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType ServiceCommon
	raw := &RawType{}
	if err := value.Decode(raw); err != nil {
		return ServiceCommon{}, err
	}

	// Set the custom unmarshalled types
	raw.HealthCheckPortVal = healthCheckPortVal

	return ServiceCommon(*raw), nil
}

func (s *ServiceCommon) healthCheckPort() *uint64 {
	if s.HealthCheckPortVal == nil {
		return nil
	}

	portS, err := s.HealthCheckPortVal.GetValue(context.Background())
	if err != nil {
		panic("failed to obtain health check port from admin api config")
	}

	port, err := strconv.ParseUint(portS, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse health check port '%s' from admin api config", portS))
	}

	return &port
}

type ServiceHttp struct {
	ServiceCommon
	PortVal    StringValue `json:"port" yaml:"port"`
	DomainVal  string      `json:"domain" yaml:"domain"`
	IsHttpsVal bool        `json:"https" yaml:"https"`
	CorsVal    *CorsConfig `json:"cors,omitempty" yaml:"cors,omitempty"`
	TlsVal     TlsConfig   `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func httpServiceUnmarshalYAML(value *yaml.Node) (ServiceHttp, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return ServiceHttp{}, fmt.Errorf("httpService expected a mapping node, got %s", KindToString(value.Kind))
	}

	cs, err := commonServiceUnmarshalYAML(value)
	if err != nil {
		return ServiceHttp{}, err
	}

	var portVal StringValue = &StringValueDirect{Value: "0"}
	var healthCheckPortVal StringValue = nil
	var tlsConfig TlsConfig

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
				return ServiceHttp{}, err
			}
			matched = true
		case "health_check_port":
			if healthCheckPortVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return ServiceHttp{}, err
			}
		case "tls":
			if tlsConfig, err = tlsConfigUnmarshalYAML(valueNode); err != nil {
				return ServiceHttp{}, err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType ServiceHttp
	raw := &RawType{}
	if err := value.Decode(raw); err != nil {
		return ServiceHttp{}, err
	}

	// Set the custom unmarshalled types
	raw.ServiceCommon = cs
	raw.PortVal = portVal
	raw.HealthCheckPortVal = healthCheckPortVal
	raw.TlsVal = tlsConfig

	return ServiceHttp(*raw), nil
}

func (s *ServiceHttp) Port() uint64 {
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

func (s *ServiceHttp) HealthCheckPort() uint64 {
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

func (s *ServiceHttp) IsHttps() bool {
	return s.IsHttpsVal
}

func (s *ServiceHttp) Domain() string {
	return s.DomainVal
}

func (s *ServiceHttp) GetBaseUrl() string {
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

func (s *ServiceHttp) TlsConfig() (*tls.Config, error) {
	return nil, nil
}

// GetServerAndHealthChecker returns a configured HTTP server based on the handler provided along with the configuration
// specified in this object. Outside logic should combine the health checker into the server if they share the same
// port.
func (s *ServiceHttp) GetServerAndHealthChecker(
	server http.Handler,
	healthChecker http.Handler,
) (httpServer *http.Server, httpHealthChecker *http.Server, err error) {
	tlsConfig, err := s.TlsConfig()
	if err != nil {
		return nil, nil, err
	}

	httpServer = &http.Server{
		Addr:      fmt.Sprintf(":%d", s.Port()),
		TLSConfig: tlsConfig,
		Handler:   server,
	}

	if s.Port() != s.HealthCheckPort() && healthChecker != nil && healthChecker != server {
		httpHealthChecker = &http.Server{
			Addr:    fmt.Sprintf(":%d", s.HealthCheckPort()),
			Handler: healthChecker,
		}
	}

	return httpServer, httpHealthChecker, nil
}
