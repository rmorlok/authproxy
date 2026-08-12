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
	if err := validateYAMLMappingFields(value, httpServiceYAMLFields...); err != nil {
		return err
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	s.ServiceHttp = hs

	return nil
}

func (s *ServiceApi) SupportsSession() bool {
	return false
}

func (s *ServiceApi) GetId() ServiceId {
	return ServiceIdApi
}

var _ HttpService = (*ServiceApi)(nil)
