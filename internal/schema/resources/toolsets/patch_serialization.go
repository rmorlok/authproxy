package toolsets

import (
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// UnmarshalJSON rejects writes to observed status and timestamps before pointer
// decoding can erase explicit nulls. Other metadata fields retain the shared
// ObjectMetaPatch null semantics. Failed decoding does not modify the receiver.
func (p *ToolSetPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool set patch must be an object")
	}
	var decoded ToolSetPatch
	for name, raw := range fields {
		var destination any
		switch name {
		case "apiVersion":
			destination = &decoded.APIVersion
		case "kind":
			destination = &decoded.Kind
		case "metadata":
			var metadata map[string]json.RawMessage
			if err := util.DecodeJSONStrict(raw, &metadata); err != nil {
				return fmt.Errorf("decode metadata: %w", err)
			}
			for key := range metadata {
				switch key {
				case "id", "name", "namespace", "generation", "labels", "annotations":
				case "createdAt", "updatedAt":
					return fmt.Errorf("metadata.%s is server-owned", key)
				default:
					return fmt.Errorf("unknown tool set metadata patch field %q", key)
				}
			}
			destination = &decoded.Metadata
		case "spec":
			destination = &decoded.Spec
		case "status":
			return fmt.Errorf("status is server-owned")
		default:
			return fmt.Errorf("unknown tool set patch field %q", name)
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
	}
	*p = decoded
	return nil
}

// UnmarshalYAML resolves aliases and merges before checking the full envelope,
// including explicit nulls in server-owned status and metadata timestamp keys.
func (p *ToolSetPatch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}

// MarshalJSON preserves omitted fields and explicit nulls for later validation.
func (p ToolSetSpecPatch) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any)
	if p.HasConnectionSelector() {
		fields["connectionSelector"] = p.ConnectionSelector
	}
	if p.HasRelease() {
		fields["release"] = p.Release
	}
	if p.HasDefinition() {
		fields["definition"] = p.Definition
	}
	return json.Marshal(fields)
}

// UnmarshalJSON strictly decodes canonical names and retains explicit null
// presence. Assignment is atomic, so failed decoding preserves the receiver.
func (p *ToolSetSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool set spec patch must be an object")
	}
	var decoded ToolSetSpecPatch
	for name, raw := range fields {
		var destination any
		switch name {
		case "connectionSelector":
			destination = &decoded.ConnectionSelector
			decoded.connectionSelectorPresent = true
		case "release":
			destination = &decoded.Release
			decoded.releasePresent = true
		case "definition":
			destination = &decoded.Definition
			decoded.definitionPresent = true
		default:
			return fmt.Errorf("unknown tool set spec patch field %q", name)
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
	}
	*p = decoded
	return nil
}

// MarshalYAML uses JSON's presence representation while retaining structured
// schemas and exact numeric values in the emitted YAML nodes.
func (p ToolSetSpecPatch) MarshalYAML() (any, error) {
	raw, err := p.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return common.RawJSON(raw).MarshalYAML()
}

// UnmarshalYAML resolves aliases and merge keys before applying JSON presence
// rules, preventing an aliased null from silently becoming an omitted field.
func (p *ToolSetSpecPatch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}

// MarshalJSON preserves omission versus an invalid explicit null desired state.
func (p ToolSetReleaseSpecPatch) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any)
	if p.HasDesiredState() {
		fields["desiredState"] = p.DesiredState
	}
	return json.Marshal(fields)
}

// UnmarshalJSON accepts only the canonical desiredState field and assigns the
// decoded value atomically, retaining null presence for validation.
func (p *ToolSetReleaseSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool set release patch must be an object")
	}
	var decoded ToolSetReleaseSpecPatch
	for name, raw := range fields {
		if name != "desiredState" {
			return fmt.Errorf("unknown tool set release patch field %q", name)
		}
		if err := util.DecodeJSONStrict(raw, &decoded.DesiredState); err != nil {
			return fmt.Errorf("decode desiredState: %w", err)
		}
		decoded.desiredStatePresent = true
	}
	*p = decoded
	return nil
}

// MarshalYAML emits the same omitted or explicit desired-state value as JSON.
func (p ToolSetReleaseSpecPatch) MarshalYAML() (any, error) {
	raw, err := p.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return common.RawJSON(raw).MarshalYAML()
}

// UnmarshalYAML resolves aliases and merges before recording field presence.
func (p *ToolSetReleaseSpecPatch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}
