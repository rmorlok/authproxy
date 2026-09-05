package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// ConnectionConfigurationJSONSchema returns a JSON Schema describing the
// connector-authored values that can be persisted in a Connection's
// spec.configuration. Auth-method-emitted credential fields are not part of
// SetupFlow and therefore are intentionally excluded.
//
// Step-level required constraints are intentionally omitted. A Connection may
// be returned while setup is incomplete, and conditional setup steps may never
// run. Additional properties remain allowed because connector migration hooks
// can add configuration fields that have no form schema. Each setup submission
// remains subject to its original step schema.
func (c *ConnectorDefinition) ConnectionConfigurationJSONSchema() (common.RawJSON, error) {
	properties := make(map[string]json.RawMessage)
	if c != nil && c.SetupFlow != nil {
		if err := mergeSetupFlowConfigurationProperties(properties, c.SetupFlow.Preconnect); err != nil {
			return nil, fmt.Errorf("preconnect configuration schema: %w", err)
		}
		if err := mergeSetupFlowConfigurationProperties(properties, c.SetupFlow.Configure); err != nil {
			return nil, fmt.Errorf("configure configuration schema: %w", err)
		}
	}

	aggregate := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}

	encoded, err := json.Marshal(aggregate)
	if err != nil {
		return nil, fmt.Errorf("marshal aggregate configuration schema: %w", err)
	}
	return common.RawJSON(encoded), nil
}

func mergeSetupFlowConfigurationProperties(
	destination map[string]json.RawMessage,
	phase *SetupFlowPhase,
) error {
	if phase == nil {
		return nil
	}

	for i := range phase.Steps {
		step := &phase.Steps[i]
		if step.Type.Normalized() != SetupFlowStepTypeForm || step.JsonSchema.IsEmpty() {
			continue
		}

		var parsed struct {
			Properties map[string]json.RawMessage `json:"properties"`
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
	}

	return nil
}

func jsonSchemaEqual(left, right json.RawMessage) bool {
	var leftCompact bytes.Buffer
	var rightCompact bytes.Buffer
	if json.Compact(&leftCompact, left) != nil || json.Compact(&rightCompact, right) != nil {
		return bytes.Equal(left, right)
	}
	return bytes.Equal(leftCompact.Bytes(), rightCompact.Bytes())
}
