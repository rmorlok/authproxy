package config

import (
	"context"
	"fmt"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
	"strconv"
)

type ServiceWorker struct {
	ServiceCommon
	ConcurrencyVal StringValue `json:"concurrency" yaml:"concurrency"`
}

func (s *ServiceWorker) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	sc, err := commonServiceUnmarshalYAML(value)
	if err != nil {
		return err
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
			if concurrencyVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
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
	raw.ServiceCommon = sc
	raw.ConcurrencyVal = concurrencyVal

	return nil
}

func (s *ServiceWorker) HealthCheckPort() uint64 {
	p := s.ServiceCommon.healthCheckPort()
	if p != nil {
		return *p
	}

	return 0
}

func (s *ServiceWorker) GetId() ServiceId {
	return ServiceIdWorker
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
