package config

import (
	"fmt"

	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type ConfiguredActorsList []*actorschema.Actor

func (cal ConfiguredActorsList) All() []*actorschema.Actor {
	return cal
}

func (cal ConfiguredActorsList) GetByExternalId(externalId string) (*actorschema.Actor, bool) {
	for _, actor := range cal {
		if actor.Spec.ExternalId == externalId {
			return actor, true
		}
	}

	return nil, false
}

func (cal ConfiguredActorsList) GetBySubject(subject string) (*actorschema.Actor, bool) {
	// Subject is the same as ExternalId (no admin/ prefix handling)
	return cal.GetByExternalId(subject)
}

func UnmarshallYamlConfiguredActorsListString(data string) (ConfiguredActorsList, error) {
	return UnmarshallYamlConfiguredActorsList([]byte(data))
}

func UnmarshallYamlConfiguredActorsList(data []byte) (ConfiguredActorsList, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return configuredActorsListUnmarshalYAML(rootNode.Content[0])
}

// configuredActorsListUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func configuredActorsListUnmarshalYAML(value *yaml.Node) (ConfiguredActorsList, error) {
	// Ensure the node is a sequence node
	if value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("actors list expected a sequence node, got %v", value.Kind)
	}

	var results []*actorschema.Actor
	for _, childNode := range value.Content {
		var actor actorschema.Actor
		if err := util.DecodeYAMLNodeStrict(childNode, &actor); err != nil {
			return nil, err
		}
		results = append(results, &actor)
	}

	return ConfiguredActorsList(results), nil
}
