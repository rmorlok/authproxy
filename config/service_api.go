package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type ServiceApi struct {
	ServiceHttp
}

func (s *ServiceApi) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	// Let the rest unmarshall normally
	type RawType ServiceApi
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ServiceHttp = hs

	return nil
}

func (s *ServiceApi) SupportsSession() bool {
	return false
}

func (s *ServiceApi) GetId() ServiceId {
	return ServiceIdApi
}

var _ HttpService = (*ServiceApi)(nil)
