package config

import (
	"fmt"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/yaml.v3"
	"strconv"
	"time"
)

type ServiceWorker struct {
	PortVal        uint64      `json:"port" yaml:"port"`
	ConcurrencyVal StringValue `json:"concurrency" yaml:"concurrency"`
}

func (s *ServiceWorker) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	var concurrencyVal StringValue = &StringValueDirect{Value: "0"}

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "concurrency":
			if concurrencyVal, err = stringValueUnmarshalYAML(valueNode); err != nil {
				return err
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
	type RawType ServiceWorker
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ConcurrencyVal = concurrencyVal

	return nil
}

func (s *ServiceWorker) Port() uint64 {
	return s.PortVal
}

func (s *ServiceWorker) IsHttps() bool {
	return false
}

func (s *ServiceWorker) Domain() string {
	return ""
}

func (s *ServiceWorker) GetBaseUrl() string {
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

func (s *ServiceWorker) SupportsSession() bool {
	return false
}

func (s *ServiceWorker) GetId() ServiceId {
	return ServiceIdWorker
}

func (s *ServiceWorker) SessionTimeout() time.Duration {
	return 0
}

func (s *ServiceWorker) CookieDomain() string {
	return ""
}

func (s *ServiceWorker) XsrfRequestQueueDepth() int {
	return 0
}

func (s *ServiceWorker) GetConcurrency(ctx context.Context) int {
	val, err := s.ConcurrencyVal.GetValue(ctx)
	if err != nil {
		return 0
	}

	parsedVal, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}

	return parsedVal
}

var _ Service = (*ServicePublic)(nil)
