package key

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// keySpecPatchWire reuses the canonical fields without their serialization methods.
// Fresh values keep decoding atomic and reset omitted fields and presence flags.
type keySpecPatchWire KeySpecPatch

// MarshalJSON preserves explicit null updates while omitting unsupplied fields.
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

// UnmarshalJSON strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (p *KeySpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire keySpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	*p = KeySpecPatch(wire)
	_, p.keyDataPresent = fields["keyData"]
	return nil
}

// MarshalYAML builds a presence-aware representation of the supplied fields.
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

// UnmarshalYAML strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (p *KeySpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire keySpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	*p = KeySpecPatch(wire)
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
