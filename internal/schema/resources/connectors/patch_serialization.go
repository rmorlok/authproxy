package connectors

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// connectorSpecPatchWire reuses the canonical fields without their serialization methods.
// Fresh values keep decoding atomic and reset omitted fields and presence flags.
type connectorSpecPatchWire ConnectorSpecPatch

// MarshalJSON preserves explicit null updates while omitting unsupplied fields.
func (c ConnectorSpecPatch) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if c.HasRelease() {
		value["release"] = c.Release
	}
	if c.HasDefinition() {
		value["definition"] = c.Definition
	}
	return json.Marshal(value)
}

// UnmarshalJSON strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (c *ConnectorSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire connectorSpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	*c = ConnectorSpecPatch(wire)
	_, c.releasePresent = fields["release"]
	_, c.definitionPresent = fields["definition"]
	return nil
}

// MarshalYAML builds a presence-aware representation of the supplied fields.
func (c ConnectorSpecPatch) MarshalYAML() (any, error) {
	value := map[string]any{}
	if c.HasRelease() {
		value["release"] = c.Release
	}
	if c.HasDefinition() {
		value["definition"] = c.Definition
	}
	return value, nil
}

// UnmarshalYAML strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (c *ConnectorSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire connectorSpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	*c = ConnectorSpecPatch(wire)
	node := value
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			switch node.Content[i].Value {
			case "release":
				c.releasePresent = true
			case "definition":
				c.definitionPresent = true
			}
		}
	}
	return nil
}

// connectorReleaseSpecPatchWire avoids recursive decoding while retaining canonical tags.
type connectorReleaseSpecPatchWire ConnectorReleaseSpecPatch

// MarshalJSON preserves explicit null updates while omitting unsupplied fields.
func (c ConnectorReleaseSpecPatch) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if c.HasDesiredState() {
		value["desiredState"] = c.DesiredState
	}
	return json.Marshal(value)
}

// UnmarshalJSON strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (c *ConnectorReleaseSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire connectorReleaseSpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	*c = ConnectorReleaseSpecPatch(wire)
	_, c.desiredStatePresent = fields["desiredState"]
	return nil
}

// MarshalYAML builds a presence-aware representation of the supplied fields.
func (c ConnectorReleaseSpecPatch) MarshalYAML() (any, error) {
	value := map[string]any{}
	if c.HasDesiredState() {
		value["desiredState"] = c.DesiredState
	}
	return value, nil
}

// UnmarshalYAML strictly decodes fields and records their presence, replacing
// the receiver only after decoding succeeds.
func (c *ConnectorReleaseSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire connectorReleaseSpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	*c = ConnectorReleaseSpecPatch(wire)
	node := value
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "desiredState" {
				c.desiredStatePresent = true
				break
			}
		}
	}
	return nil
}
