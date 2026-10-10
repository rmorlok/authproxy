package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// UnmarshalJSON accepts canonical definition fields and rejects null policy
// values before slice decoding can erase their presence. Assignment is atomic.
func (d *ToolSetDefinition) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool set definition must be an object")
	}
	var decoded ToolSetDefinition
	for name, raw := range fields {
		var destination any
		switch name {
		case "source":
			destination = &decoded.Source
		case "permissionMappings":
			destination = &decoded.PermissionMappings
		default:
			return fmt.Errorf("unknown tool set definition field %q", name)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", name)
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
	}
	*d = decoded
	return nil
}

// MarshalYAML follows JSON's null handling for required nested slices. Native
// YAML encoding would turn an invalid nil explicit inventory into valid [].
func (d ToolSetDefinition) MarshalYAML() (any, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return common.RawJSON(raw).MarshalYAML()
}

// UnmarshalYAML resolves aliases and merges before applying strict JSON rules.
func (d *ToolSetDefinition) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return d.UnmarshalJSON(raw)
}

// UnmarshalJSON keeps invalid null branches from disappearing beside another
// source. Exactly-one validation is separate so malformed authored shapes can
// still receive complete field diagnostics from Validate.
func (s *ToolSetSource) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool set source must be an object")
	}
	var decoded ToolSetSource
	for name, raw := range fields {
		var destination any
		switch name {
		case "explicit":
			destination = &decoded.Explicit
		case "openapi":
			destination = &decoded.OpenAPI
		case "mcp":
			destination = &decoded.MCP
		default:
			return fmt.Errorf("unknown tool set source field %q", name)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("source.%s must not be null", name)
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode source.%s: %w", name, err)
		}
	}
	*s = decoded
	return nil
}

// UnmarshalYAML applies the same union boundary to direct YAML source decoding.
func (s *ToolSetSource) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}
