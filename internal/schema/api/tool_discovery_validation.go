package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// Validate checks the public descriptor shape and detail-level consistency. It
// does not authorize, redact text, compile schemas, or infer runtime availability.
func (d *ToolDescriptor) Validate(vc *common.ValidationContext) error {
	vc = discoveryValidationContext(vc)

	if d == nil {
		return vc.NewError("tool descriptor is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateTypeMeta(d.TypeMeta, meta.APIVersionV1Alpha1, ToolDescriptorKind, vc))

	if err := toolschema.ValidateID(d.ToolID); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("toolId", err.Error()))
	}

	if d.Revision == 0 {
		result = multierror.Append(result, vc.NewErrorForField("revision", "must be greater than zero"))
	}

	for field, value := range map[string]string{"name": d.Name, "description": d.Description} {
		if strings.TrimSpace(value) == "" {
			result = multierror.Append(result, vc.NewErrorForField(field, "must not be blank"))
		}
	}

	result = multierror.Append(result, d.Connection.Validate(vc.PushField("connection")))

	if d.ToolSet != nil {
		path := vc.PushField("toolSet")

		if err := toolsetschema.ValidateID(d.ToolSet.ID); err != nil {
			result = multierror.Append(result, path.NewErrorForField("id", err.Error()))
		}

		if strings.TrimSpace(d.ToolSet.Name) == "" {
			result = multierror.Append(result, path.NewErrorForField("name", "must not be blank"))
		}

		if d.ToolSet.Generation == 0 {
			result = multierror.Append(result, path.NewErrorForField("generation", "must be greater than zero"))
		}
	}

	if d.CatalogRevision != nil {
		if *d.CatalogRevision == 0 {
			result = multierror.Append(result, vc.NewErrorForField("catalogRevision", "must be greater than zero"))
		}

		if d.ToolSet == nil {
			result = multierror.Append(result, vc.NewErrorForField("toolSet", "is required with catalogRevision"))
		}
	}

	switch d.Readiness.State {
	case ToolReadinessStateReady, ToolReadinessStateUnavailable, ToolReadinessStateUnknown:
	default:
		result = multierror.Append(result, vc.NewErrorForField("readiness.state", "must be ready, unavailable, or unknown"))
	}

	if d.Readiness.Reason != "" && strings.TrimSpace(d.Readiness.Reason) == "" {
		result = multierror.Append(result, vc.NewErrorForField("readiness.reason", "must not be blank when supplied"))
	}

	if d.ConnectionHealth != nil && !connectionschema.IsValidConnectionHealthState(d.ConnectionHealth.State) {
		result = multierror.Append(result, vc.NewErrorForField("connectionHealth.state", "must be healthy or unhealthy"))
	}

	if d.Hints != nil {
		for field, hint := range map[string]*ToolHint{
			"readOnly": d.Hints.ReadOnly, "destructive": d.Hints.Destructive, "idempotent": d.Hints.Idempotent,
		} {
			if hint != nil && hint.Provenance != ToolHintProvenanceAdministrator && hint.Provenance != ToolHintProvenanceProvider {
				result = multierror.Append(result, vc.PushField("hints").PushField(field).NewErrorForField("provenance", "must be administrator or provider"))
			}
		}
	}

	result = multierror.Append(result, d.validateDetail(vc))

	return result.ErrorOrNil()
}

// validateDetail checks which fields a descriptor may carry without pruning or
// interpreting JSON Schema keywords. Schema compilation and constraint-preserving
// compact projections are the producer's responsibility.
func (d *ToolDescriptor) validateDetail(vc *common.ValidationContext) error {
	var result *multierror.Error

	switch d.Schema {
	case ToolSchemaDetailNone:
		if len(d.InputSchema) != 0 || len(d.OutputSchema) != 0 {
			result = multierror.Append(result, vc.NewErrorForField("schema", "none must omit inputSchema and outputSchema"))
		}
	case ToolSchemaDetailCompact, ToolSchemaDetailFull:
		if len(d.InputSchema) == 0 {
			result = multierror.Append(result, vc.NewErrorForField("inputSchema", "is required for compact or full detail"))
		} else if err := validateOptionalToolJSON(d.InputSchema, true); err != nil {
			result = multierror.Append(result, vc.NewErrorForField("inputSchema", err.Error()))
		}

		if len(d.OutputSchema) != 0 {
			if err := validateDescriptorOutputSchema(d.OutputSchema); err != nil {
				result = multierror.Append(result, vc.NewErrorForField("outputSchema", err.Error()))
			}
		}
	default:
		result = multierror.Append(result, vc.NewErrorForField("schema", "must be none, compact, or full"))
	}

	if d.Schema != ToolSchemaDetailFull && (d.Usage != "" || len(d.Examples) != 0) {
		result = multierror.Append(result, vc.NewErrorForField("schema", "usage and examples require full detail"))
	}

	if len(d.Examples) != 0 {
		if err := validateDescriptorExamples(d.Examples); err != nil {
			result = multierror.Append(result, vc.NewErrorForField("examples", err.Error()))
		}
	}

	return result.ErrorOrNil()
}

// validateDescriptorOutputSchema checks only the JSON Schema document shape;
// true and false are meaningful schemas, whereas null is not a schema.
func validateDescriptorOutputSchema(raw common.RawJSON) error {
	if !json.Valid(raw) {
		return fmt.Errorf("must contain one valid JSON value")
	}

	trimmed := bytes.TrimSpace(raw)

	if trimmed[0] != '{' && !bytes.Equal(trimmed, []byte("true")) && !bytes.Equal(trimmed, []byte("false")) {
		return fmt.Errorf("must contain a JSON object or boolean schema")
	}

	return nil
}

// validateDescriptorExamples requires an array of invocation argument objects.
// Decoding into raw messages avoids changing nested values or number precision.
func validateDescriptorExamples(raw common.RawJSON) error {
	var examples []json.RawMessage

	if err := json.Unmarshal(raw, &examples); err != nil || examples == nil {
		return fmt.Errorf("must contain an array of argument objects")
	}

	for i, example := range examples {
		if err := validateOptionalToolJSON(common.RawJSON(example), true); err != nil {
			return fmt.Errorf("example %d: %w", i, err)
		}
	}

	return nil
}

// Validate checks the minimal connection identity without resolving access.
func (c *ToolConnectionIdentity) Validate(vc *common.ValidationContext) error {
	vc = discoveryValidationContext(vc)

	if c == nil {
		return vc.NewError("connection identity is required")
	}

	var result *multierror.Error

	if err := connectionschema.ValidateID(c.ID); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("id", err.Error()))
	}

	if strings.TrimSpace(c.Name) == "" {
		result = multierror.Append(result, vc.NewErrorForField("name", "must not be blank"))
	}

	return result.ErrorOrNil()
}

// Validate checks the connection projection. CanProxy is a server-computed
// actor/JWT intersection, which shape validation cannot independently verify.
func (c *AgentConnectionSummary) Validate(vc *common.ValidationContext) error {
	vc = discoveryValidationContext(vc)

	if c == nil {
		return vc.NewError("agent connection summary is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateTypeMeta(c.TypeMeta, meta.APIVersionV1Alpha1, AgentConnectionKind, vc))
	result = multierror.Append(result, c.ToolConnectionIdentity.Validate(vc))

	return result.ErrorOrNil()
}

// Validate checks pagination metadata and every descriptor in a response page.
// Empty pages must serialize as [] rather than null.
func (l *ToolDescriptorList) Validate() error {
	if l == nil {
		return fmt.Errorf("$: tool descriptor list is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, l.ResourceList.Validate(ToolDescriptorKind))

	if l.Items == nil {
		result = multierror.Append(result, fmt.Errorf("$.items: must be an array"))
	}

	for i := range l.Items {
		result = multierror.Append(result, l.Items[i].Validate((&common.ValidationContext{Path: "$.items"}).PushIndex(i)))
	}

	return result.ErrorOrNil()
}

// Validate checks the authorized connection page shape, without deriving proxy
// capability from connection visibility or tool readiness.
func (l *AgentConnectionList) Validate() error {
	if l == nil {
		return fmt.Errorf("$: agent connection list is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, l.ResourceList.Validate(AgentConnectionKind))

	if l.Items == nil {
		result = multierror.Append(result, fmt.Errorf("$.items: must be an array"))
	}

	for i := range l.Items {
		result = multierror.Append(result, l.Items[i].Validate((&common.ValidationContext{Path: "$.items"}).PushIndex(i)))
	}

	return result.ErrorOrNil()
}

// discoveryValidationContext supplies the root path for standalone DTO checks.
func discoveryValidationContext(vc *common.ValidationContext) *common.ValidationContext {
	if vc == nil {
		return &common.ValidationContext{Path: "$"}
	}

	return vc
}
