package connectors

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type connectorSpecPatchWire struct {
	Release    *ConnectorReleaseSpecPatch `json:"release,omitempty" yaml:"release,omitempty"`
	Definition *ConnectorDefinition       `json:"definition,omitempty" yaml:"definition,omitempty"`
}

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

func (c *ConnectorSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire connectorSpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	*c = ConnectorSpecPatch{Release: wire.Release, Definition: wire.Definition}
	_, c.releasePresent = fields["release"]
	_, c.definitionPresent = fields["definition"]
	return nil
}

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

func (c *ConnectorSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire connectorSpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	*c = ConnectorSpecPatch{Release: wire.Release, Definition: wire.Definition}
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

func (c ConnectorReleaseSpecPatch) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if c.HasDesiredState() {
		value["desiredState"] = c.DesiredState
	}
	return json.Marshal(value)
}

func (c *ConnectorReleaseSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire struct {
		DesiredState *ConnectorReleaseState `json:"desiredState,omitempty"`
	}
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	*c = ConnectorReleaseSpecPatch{DesiredState: wire.DesiredState}
	_, c.desiredStatePresent = fields["desiredState"]
	return nil
}

func (c ConnectorReleaseSpecPatch) MarshalYAML() (any, error) {
	value := map[string]any{}
	if c.HasDesiredState() {
		value["desiredState"] = c.DesiredState
	}
	return value, nil
}

func (c *ConnectorReleaseSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire struct {
		DesiredState *ConnectorReleaseState `yaml:"desiredState,omitempty"`
	}
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	*c = ConnectorReleaseSpecPatch{DesiredState: wire.DesiredState}
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
