package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type ServiceCommon struct {
	HealthCheckPortVal *IntegerValue `json:"healthCheckPort,omitempty" yaml:"healthCheckPort,omitempty"`
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
	BaseUrl       *StringValue  `json:"baseUrl,omitempty" yaml:"baseUrl,omitempty"`
	DomainVal     string        `json:"domain" yaml:"domain"`
	IsHttpsVal    bool          `json:"https" yaml:"https"`
	CorsVal       *CorsConfig   `json:"cors,omitempty" yaml:"cors,omitempty"`
	TlsVal        TlsConfig     `json:"tls,omitempty" yaml:"tls,omitempty"`
}

var httpServiceYAMLFields = []string{
	"healthCheckPort",
	"port",
	"baseUrl",
	"domain",
	"https",
	"cors",
	"tls",
}

// validateYAMLMappingFields ensures custom YAML unmarshalling retains the
// unknown-field behavior used by the ordinary strict decoder.
func validateYAMLMappingFields(value *yaml.Node, fieldNames ...string) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node, got %s", KindToString(value.Kind))
	}

	allowed := make(map[string]struct{}, len(fieldNames))
	for _, fieldName := range fieldNames {
		allowed[fieldName] = struct{}{}
	}

	for i := 0; i < len(value.Content); i += 2 {
		if _, ok := allowed[value.Content[i].Value]; !ok {
			return fmt.Errorf("unknown config field %q", value.Content[i].Value)
		}
	}

	return nil
}

// yamlMappingWithFields returns a shallow mapping view containing only the
// requested fields. It lets embedded custom unmarshallers decode their own
// fields strictly without seeing their parent's fields.
func yamlMappingWithFields(value *yaml.Node, fieldNames ...string) *yaml.Node {
	fields := make(map[string]struct{}, len(fieldNames))
	for _, fieldName := range fieldNames {
		fields[fieldName] = struct{}{}
	}

	filtered := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i < len(value.Content); i += 2 {
		if _, ok := fields[value.Content[i].Value]; ok {
			filtered.Content = append(filtered.Content, value.Content[i], value.Content[i+1])
		}
	}

	return filtered
}

func httpServiceUnmarshalYAML(value *yaml.Node) (ServiceHttp, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return ServiceHttp{}, fmt.Errorf("httpService expected a mapping node, got %s", KindToString(value.Kind))
	}

	var tlsConfig TlsConfig

	// Handle custom unmarshalling for TLS without changing the parent mapping:
	// ServicePublic and ServiceAdminApi contain additional fields that must be
	// decoded by their own strict unmarshallers.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "tls":
			var err error
			if tlsConfig, err = tlsConfigUnmarshalYAML(valueNode); err != nil {
				return ServiceHttp{}, err
			}
		}
	}

	// Decode only the common HTTP fields. TlsVal is an interface and is handled
	// above, so deliberately omit it from the raw type.
	type rawServiceHttp struct {
		HealthCheckPortVal *IntegerValue `yaml:"healthCheckPort,omitempty"`
		PortVal            *IntegerValue `yaml:"port"`
		BaseUrl            *StringValue  `yaml:"baseUrl,omitempty"`
		DomainVal          string        `yaml:"domain"`
		IsHttpsVal         bool          `yaml:"https"`
		CorsVal            *CorsConfig   `yaml:"cors,omitempty"`
	}

	raw := &rawServiceHttp{}
	httpFields := yamlMappingWithFields(value, httpServiceYAMLFields[:len(httpServiceYAMLFields)-1]...)
	if err := util.DecodeYAMLNodeStrict(httpFields, raw); err != nil {
		return ServiceHttp{}, err
	}

	return ServiceHttp{
		ServiceCommon: ServiceCommon{HealthCheckPortVal: raw.HealthCheckPortVal},
		PortVal:       raw.PortVal,
		BaseUrl:       raw.BaseUrl,
		DomainVal:     raw.DomainVal,
		IsHttpsVal:    raw.IsHttpsVal,
		CorsVal:       raw.CorsVal,
		TlsVal:        tlsConfig,
	}, nil
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
	ctx := context.Background()
	if s.BaseUrl != nil && s.BaseUrl.HasValue(ctx) {
		baseUrl, err := s.BaseUrl.GetValue(ctx)
		if err == nil && baseUrl != "" {
			return strings.TrimRight(baseUrl, "/")
		}
	}

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
