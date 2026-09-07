package key

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type keySpecPatchWire struct {
	Usage        *KeyUsage        `json:"usage,omitempty" yaml:"usage,omitempty"`
	MaterialType *KeyMaterialType `json:"materialType,omitempty" yaml:"materialType,omitempty"`
	DesiredState *KeyState        `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`
	KeyData      *KeyData         `json:"keyData,omitempty" yaml:"keyData,omitempty"`
}

func (p KeySpecPatch) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if p.Usage != nil {
		value["usage"] = p.Usage
	}
	if p.MaterialType != nil {
		value["materialType"] = p.MaterialType
	}
	if p.DesiredState != nil {
		value["desiredState"] = p.DesiredState
	}
	if p.HasKeyData() {
		value["keyData"] = p.KeyData
	}
	return json.Marshal(value)
}

func (p *KeySpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire keySpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	p.Usage = wire.Usage
	p.MaterialType = wire.MaterialType
	p.DesiredState = wire.DesiredState
	p.KeyData = wire.KeyData
	_, p.keyDataPresent = fields["keyData"]
	return nil
}

func (p KeySpecPatch) MarshalYAML() (any, error) {
	value := map[string]any{}
	if p.Usage != nil {
		value["usage"] = p.Usage
	}
	if p.MaterialType != nil {
		value["materialType"] = p.MaterialType
	}
	if p.DesiredState != nil {
		value["desiredState"] = p.DesiredState
	}
	if p.HasKeyData() {
		value["keyData"] = p.KeyData
	}
	return value, nil
}

func (p *KeySpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire keySpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	p.Usage = wire.Usage
	p.MaterialType = wire.MaterialType
	p.DesiredState = wire.DesiredState
	p.KeyData = wire.KeyData
	p.keyDataPresent = false
	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content); i += 2 {
			if value.Content[i].Value == "keyData" {
				p.keyDataPresent = true
				break
			}
		}
	}
	return nil
}
