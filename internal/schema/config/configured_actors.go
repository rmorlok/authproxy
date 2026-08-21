package config

import (
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type ConfiguredActorsType interface {
	All() []*ConfiguredActor
	GetByExternalId(externalId string) (*ConfiguredActor, bool)
	GetBySubject(subject string) (*ConfiguredActor, bool)
}

type ConfiguredActors struct {
	InnerVal ConfiguredActorsType `json:"-" yaml:"-"`
}

func (ca *ConfiguredActors) All() []*ConfiguredActor {
	if ca == nil || ca.InnerVal == nil {
		return nil
	}
	return ca.InnerVal.All()
}

func (ca *ConfiguredActors) GetByExternalId(externalId string) (*ConfiguredActor, bool) {
	if ca == nil || ca.InnerVal == nil {
		return nil, false
	}
	return ca.InnerVal.GetByExternalId(externalId)
}

func (ca *ConfiguredActors) GetBySubject(subject string) (*ConfiguredActor, bool) {
	if ca == nil || ca.InnerVal == nil {
		return nil, false
	}
	return ca.InnerVal.GetBySubject(subject)
}

func (ca *ConfiguredActors) MarshalJSON() ([]byte, error) {
	if ca.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(ca.InnerVal)
}

func (ca *ConfiguredActors) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '[' && data[len(data)-1] == ']' {
		var actorsList ConfiguredActorsList
		err := util.DecodeJSONStrict(data, &actorsList)
		ca.InnerVal = actorsList
		return err
	}

	var sources ConfiguredActorsExternalSources
	if err := util.DecodeJSONStrict(data, &sources); err != nil {
		return err
	}
	ca.InnerVal = &sources

	return nil
}

func (ca *ConfiguredActors) MarshalYAML() (interface{}, error) {
	if ca.InnerVal == nil {
		return nil, nil
	}

	return ca.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (ca *ConfiguredActors) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var actorsList ConfiguredActorsList
		err := util.DecodeYAMLNodeStrict(value, &actorsList)
		ca.InnerVal = actorsList
		return err
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("actors expected a sequence node or mapping node, got %s", KindToString(value.Kind))
	}

	var sources ConfiguredActorsExternalSources
	if err := util.DecodeYAMLNodeStrict(value, &sources); err != nil {
		return err
	}
	ca.InnerVal = &sources
	return nil
}

var _ ConfiguredActorsType = (*ConfiguredActors)(nil)
