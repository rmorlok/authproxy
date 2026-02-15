package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"gopkg.in/yaml.v3"
)

type ServiceCommon struct {
	HealthCheckPortVal *IntegerValue `json:"health_check_port,omitempty" yaml:"health_check_port,omitempty"`
}

func (s *ServiceCommon) healthCheckPort() *uint64 {
	if s.HealthCheckPortVal == nil {
		return nil
	}

	port, err := s.HealthCheckPortVal.GetUint64Value(context.Background())
	if err != nil {
		panic("failed to obtain health check port from admin api config")
	}

	return &port
}

type ServiceHttp struct {
	ServiceCommon `json:",inline" yaml:",inline"`
	PortVal       *IntegerValue `json:"port" yaml:"port"`
	DomainVal     string        `json:"domain" yaml:"domain"`
	IsHttpsVal    bool          `json:"https" yaml:"https"`
	CorsVal       *CorsConfig   `json:"cors,omitempty" yaml:"cors,omitempty"`
	TlsVal        TlsConfig     `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func httpServiceUnmarshalYAML(value *yaml.Node) (ServiceHttp, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return ServiceHttp{}, fmt.Errorf("httpService expected a mapping node, got %s", KindToString(value.Kind))
	}

	var tlsConfig TlsConfig

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "tls":
			if tlsConfig, err = tlsConfigUnmarshalYAML(valueNode); err != nil {
				return ServiceHttp{}, err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling and pull back our index
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
	raw.TlsVal = tlsConfig

	return ServiceHttp(*raw), nil
}

func (s *ServiceHttp) Port() uint64 {
	port, err := s.PortVal.GetUint64Value(context.Background())
	if err != nil {
		panic("failed to obtain port from admin api config")
	}

	return port
}

func (s *ServiceHttp) HealthCheckPort() uint64 {
	if s.HealthCheckPortVal == nil {
		return s.Port()
	}

	port, err := s.HealthCheckPortVal.GetUint64Value(context.Background())
	if err != nil {
		panic("failed to obtain health check port from admin api config")
	}

	return port
}

func (s *ServiceHttp) IsHttps() bool {
	return s.TlsVal != nil || s.IsHttpsVal
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
	if s.TlsVal == nil {
		return nil, nil
	}

	return s.TlsVal.TlsConfig(context.Background(), s)
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
