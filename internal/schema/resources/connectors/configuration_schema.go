package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
)

// ConnectionConfigurationJSONSchema returns a JSON Schema describing the
// connector-authored values that can be persisted in a Connection's
// spec.configuration. Auth-method-emitted credential fields are not part of
// SetupFlow and therefore are intentionally excluded.
//
// Required constraints from unconditional steps are retained. Requirements
// from conditionally eligible steps are omitted because their JavaScript
// predicates cannot be represented faithfully by this aggregate JSON Schema.
// Additional properties remain allowed because connector migration hooks can
// add configuration fields that have no form schema. Each setup submission
// remains subject to its original step schema.
func (c *ConnectorDefinition) ConnectionConfigurationJSONSchema() (common.RawJSON, error) {
	properties := make(map[string]json.RawMessage)
	required := make([]string, 0)
	requiredSet := make(map[string]struct{})
	if c != nil && c.SetupFlow != nil {
		if err := mergeSetupFlowConfigurationSchema(
			properties,
			&required,
			requiredSet,
			c.SetupFlow.Preconnect,
		); err != nil {
			return nil, fmt.Errorf("preconnect configuration schema: %w", err)
		}

		if err := mergeSetupFlowConfigurationSchema(
			properties,
			&required,
			requiredSet,
			c.SetupFlow.Configure,
		); err != nil {
			return nil, fmt.Errorf("configure configuration schema: %w", err)
		}
	}

	aggregate := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}
	if len(required) > 0 {
		aggregate["required"] = required
	}

	encoded, err := json.Marshal(aggregate)
	if err != nil {
		return nil, fmt.Errorf("marshal aggregate configuration schema: %w", err)
	}

	return common.RawJSON(encoded), nil
}

// ConnectionConfigurationMatchesJSONSchema reports whether configuration is
// sufficient for the aggregate schema returned by
// ConnectionConfigurationJSONSchema. A missing configuration is treated as an
// empty object so connectors without required configuration fields are
// considered configured.
func ConnectionConfigurationMatchesJSONSchema(
	schema common.RawJSON,
	configuration map[string]any,
) (bool, error) {
	compiled, err := jsonschemav5.CompileString(
		"connection-configuration.json",
		string(schema),
	)
	if err != nil {
		return false, fmt.Errorf("compile connection configuration schema: %w", err)
	}

	if configuration == nil {
		configuration = map[string]any{}
	}

	return compiled.Validate(configuration) == nil, nil
}

func mergeSetupFlowConfigurationSchema(
	destination map[string]json.RawMessage,
	required *[]string,
	requiredSet map[string]struct{},
	phase *SetupFlowPhase,
) error {
	if phase == nil {
		return nil
	}

	for i := range phase.Steps {
		step := &phase.Steps[i]
		if step.Type.Normalized() != SetupFlowStepTypeForm ||
			step.JsonSchema.IsEmpty() {
			continue
		}

		var parsed struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(step.JsonSchema, &parsed); err != nil {
			return fmt.Errorf("step %q: %w", step.Id, err)
		}

		for name, property := range parsed.Properties {
			existing, found := destination[name]
			if !found || jsonSchemaEqual(existing, property) {
				destination[name] = property
				continue
			}

			combined, err := json.Marshal(struct {
				AnyOf []json.RawMessage `json:"anyOf"`
			}{AnyOf: []json.RawMessage{existing, property}})
			if err != nil {
				return fmt.Errorf("step %q property %q: %w", step.Id, name, err)
			}

			destination[name] = combined
		}

		if step.If == nil {
			for _, name := range parsed.Required {
				if _, found := requiredSet[name]; found {
					continue
				}

				requiredSet[name] = struct{}{}
				*required = append(*required, name)
			}
		}
	}

	return nil
}

func jsonSchemaEqual(left, right json.RawMessage) bool {
	var leftCompact bytes.Buffer
	var rightCompact bytes.Buffer

	if json.Compact(&leftCompact, left) != nil ||
		json.Compact(&rightCompact, right) != nil {
		return bytes.Equal(left, right)
	}

	return bytes.Equal(leftCompact.Bytes(), rightCompact.Bytes())
}
