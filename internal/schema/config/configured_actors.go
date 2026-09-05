package config

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type ConfiguredActorsType interface {
	All() []*actorschema.Actor
	GetByExternalId(externalId string) (*actorschema.Actor, bool)
	GetBySubject(subject string) (*actorschema.Actor, bool)
}

type ConfiguredActors struct {
	InnerVal ConfiguredActorsType `json:"-" yaml:"-"`
}

func (ca *ConfiguredActors) All() []*actorschema.Actor {
	if ca == nil || ca.InnerVal == nil {
		return nil
	}
	return ca.InnerVal.All()
}

func (ca *ConfiguredActors) GetByExternalId(externalId string) (*actorschema.Actor, bool) {
	if ca == nil || ca.InnerVal == nil {
		return nil, false
	}
	return ca.InnerVal.GetByExternalId(externalId)
}

func (ca *ConfiguredActors) GetBySubject(subject string) (*actorschema.Actor, bool) {
	if ca == nil || ca.InnerVal == nil {
		return nil, false
	}
	return ca.InnerVal.GetBySubject(subject)
}

// Validate checks statically configured Actor resources. External key sources
// are materialized and validated when synchronized so configuration loading
// does not depend on the source directory being available yet.
func (ca *ConfiguredActors) Validate(vc *common.ValidationContext) error {
	if ca == nil || ca.InnerVal == nil {
		return nil
	}
	list, ok := ca.InnerVal.(ConfiguredActorsList)
	if !ok {
		return nil
	}
	var result *multierror.Error
	for i, actor := range list {
		itemContext := vc.PushIndex(i)
		if actor == nil {
			result = multierror.Append(result, itemContext.NewError("actor is required"))
			continue
		}
		if err := actor.ValidateFor(meta.ValidationModeConfig, itemContext); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result.ErrorOrNil()
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

	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var t ConfiguredActorsType

	if _, ok := valueMap["keysPath"]; ok {
		t = &ConfiguredActorsExternalSource{}
	} else {
		return fmt.Errorf("invalid structure for actors; must be list or have keysPath")
	}

	if err := util.DecodeJSONStrict(data, t); err != nil {
		return err
	}

	ca.InnerVal = t

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

	var actorsExternalSource ConfiguredActorsExternalSource
	if err := util.DecodeYAMLNodeStrict(value, &actorsExternalSource); err != nil {
		return err
	}

	ca.InnerVal = &actorsExternalSource
	return nil
}

var _ ConfiguredActorsType = (*ConfiguredActors)(nil)
