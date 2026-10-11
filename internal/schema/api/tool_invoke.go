package api

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// ToolInvokeActionKind identifies an invocation of one Tool by its immutable ID.
const ToolInvokeActionKind meta.Kind = "ToolInvoke"

// ToolInvokeSpec carries caller arguments and an optional discovery precondition.
// It does not contain resolved connection configuration or credentials.
type ToolInvokeSpec struct {
	// Arguments must be a JSON object, including {} for a tool with no inputs.
	// Keep its values intact; admission validates the captured tool's input schema.
	Arguments common.RawJSON `json:"arguments" yaml:"arguments"`

	// ExpectedRevision must match the current revision at admission when supplied.
	// Omission executes the current revision. This is neither a historical revision
	// selector nor an idempotency token; it cannot make retries safe.
	ExpectedRevision *uint64 `json:"expectedRevision,omitempty" yaml:"expectedRevision,omitempty"`
}

// ToolInvokeRequest targets a Tool by ID and supplies its invocation arguments.
// Status is server-owned and must not be authored by the caller.
type ToolInvokeRequest struct {
	apiv1alpha1.Action[ToolInvokeSpec, struct{}] `json:",inline" yaml:",inline"`
}

// ToolInvokeStatus contains the completed invocation outcome, including a tool
// error when one can be formed. Earlier HTTP failures use the API error envelope.
type ToolInvokeStatus struct {
	Result ToolResult `json:"result" yaml:"result"`
}

// ToolInvokeResponse wraps a rich result without echoing request arguments.
// Its empty spec follows the existing observation-only action response pattern.
type ToolInvokeResponse struct {
	apiv1alpha1.Action[struct{}, ToolInvokeStatus] `json:",inline" yaml:",inline"`
}

// NewToolInvokeRequest wraps caller inputs in the v1alpha1 action envelope.
// Construction does not validate the request or copy its argument bytes.
func NewToolInvokeRequest(target meta.ObjectReference, spec ToolInvokeSpec) ToolInvokeRequest {
	return ToolInvokeRequest{Action: apiv1alpha1.NewActionRequest(ToolInvokeActionKind, target, spec)}
}

// NewToolInvokeResponse wraps an invocation result with an empty response spec.
// Construction does not validate or copy the result; producers own redaction.
func NewToolInvokeResponse(target meta.ObjectReference, result ToolResult) ToolInvokeResponse {
	return ToolInvokeResponse{Action: apiv1alpha1.NewActionResponse(
		ToolInvokeActionKind,
		target,
		struct{}{},
		ToolInvokeStatus{Result: result},
	)}
}

// ValidateRequest checks the action envelope, target, and argument shape without
// coercion. Authorization, URL-target agreement, input-schema validation, and the
// actual expectedRevision comparison belong to invocation admission.
func (a *ToolInvokeRequest) ValidateRequest(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}

	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}

	if err := validateToolInvokeTarget(a.Metadata.Target); err != nil {
		return err
	}

	if len(a.Spec.Arguments) == 0 {
		return fmt.Errorf("$.spec.arguments: is required")
	}

	if err := validateOptionalToolJSON(a.Spec.Arguments, true); err != nil {
		return fmt.Errorf("$.spec.arguments: %w", err)
	}

	if a.Spec.ExpectedRevision != nil && *a.Spec.ExpectedRevision == 0 {
		return fmt.Errorf("$.spec.expectedRevision: must be greater than zero")
	}

	return nil
}

// ValidateResponse checks the action and rich result, including agreement between
// the target Tool ID and the result. It does not verify runtime execution state.
func (a *ToolInvokeResponse) ValidateResponse(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}

	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}

	if err := validateToolInvokeTarget(a.Metadata.Target); err != nil {
		return err
	}

	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}

	if err := a.Status.Result.Validate(&common.ValidationContext{Path: "$.status.result"}); err != nil {
		return err
	}

	if a.Status.Result.ToolID != a.Metadata.Target.ID {
		return fmt.Errorf("$.status.result.toolId: must match metadata.target.id")
	}

	return nil
}

// validateToolInvokeTarget requires a typed Tool ID. Mutable names cannot pin an
// invocation's identity, and revision preconditions belong in spec, not generation.
func validateToolInvokeTarget(target meta.ObjectReference) error {
	vc := &common.ValidationContext{Path: "$.metadata.target"}
	var result *multierror.Error

	if err := meta.ValidateObjectReferenceWithOptions(target, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       toolschema.ToolKind,
		IDValidator:        toolschema.ValidateID,
	}, vc); err != nil {
		result = multierror.Append(result, err)
	}

	if target.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("id", "is required for tool invocation targets"))
	}

	if target.Name != "" || target.Namespace != "" {
		result = multierror.Append(result, vc.NewError("tool invocation targets support id only"))
	}

	if target.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "does not apply to tool invocations; use spec.expectedRevision"))
	}

	return result.ErrorOrNil()
}
