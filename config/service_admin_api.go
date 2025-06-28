package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type ServiceAdminApi struct {
	ServiceHttp
}

func (s *ServiceAdminApi) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	// Let the rest unmarshall normally
	type RawType ServiceAdminApi
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ServiceHttp = hs

	return nil
}

func (s *ServiceAdminApi) SupportsSession() bool {
	return false
}

func (s *ServiceAdminApi) GetId() ServiceId {
	return ServiceIdAdminApi
}

var _ HttpService = (*ServiceAdminApi)(nil)
