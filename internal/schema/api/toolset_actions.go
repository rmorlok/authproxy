package api

import (
	"fmt"
	"strings"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// These actions target a Connection and identify one ToolSet in spec. They do
// not give the controller-owned binding an independent public CRUD identity.
const (
	ConnectionToolSetGenerationMigrationActionKind meta.Kind = "ConnectionToolSetGenerationMigration"
	ConnectionToolSetRefreshActionKind             meta.Kind = "ConnectionToolSetRefresh"
)

// ConnectionToolSetGenerationMigrationSpec selects an exact published ToolSet
// generation for one connection. An older active generation can be a rollback
// target; the service checks publication and binding eligibility.
type ConnectionToolSetGenerationMigrationSpec struct {
	ToolSetRef meta.ObjectReference `json:"toolSetRef" yaml:"toolSetRef"`

	// TimeoutSeconds bounds the background operation when supplied. Omission
	// leaves the service default in effect; this is not an HTTP wait duration.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
}

// ConnectionToolSetRefreshSpec selects one logical ToolSet binding for MCP
// catalog refresh. Its reference has no generation: refresh uses the binding's
// current source configuration without migrating the installed generation.
type ConnectionToolSetRefreshSpec struct {
	ToolSetRef meta.ObjectReference `json:"toolSetRef" yaml:"toolSetRef"`

	// TimeoutSeconds has the same background-operation meaning as migration.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
}

// ConnectionToolSetActionStatus acknowledges accepted background management
// work. It does not assert that migration or catalog activation has completed.
type ConnectionToolSetActionStatus struct {
	// TaskID is the existing opaque, actor-bound encrypted tracking token for
	// GET /tasks/{token}, not an APID or a raw Asynq/workflow identifier. The
	// producer issues it only after the operation has been accepted durably.
	TaskID string `json:"taskId" yaml:"taskId"`
}

// ConnectionToolSetGenerationMigrationAction requests or acknowledges migration
// of one connection's ToolSet binding. Requests omit server-owned status;
// responses retain the safe request spec and add a task tracking token.
type ConnectionToolSetGenerationMigrationAction struct {
	apiv1alpha1.Action[ConnectionToolSetGenerationMigrationSpec, ConnectionToolSetActionStatus] `json:",inline" yaml:",inline"`
}

// ConnectionToolSetRefreshAction requests or acknowledges an MCP catalog refresh
// for one connection. It does not refresh an OpenAPI ToolSet's draft snapshot.
type ConnectionToolSetRefreshAction struct {
	apiv1alpha1.Action[ConnectionToolSetRefreshSpec, ConnectionToolSetActionStatus] `json:",inline" yaml:",inline"`
}

// NewConnectionToolSetGenerationMigrationResponse wraps an accepted operation.
// It neither schedules work nor validates or resolves the supplied references.
func NewConnectionToolSetGenerationMigrationResponse(target meta.ObjectReference, spec ConnectionToolSetGenerationMigrationSpec, status ConnectionToolSetActionStatus) ConnectionToolSetGenerationMigrationAction {
	return ConnectionToolSetGenerationMigrationAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionToolSetGenerationMigrationActionKind, target, spec, status,
	)}
}

// NewConnectionToolSetRefreshResponse wraps an accepted catalog refresh without
// scheduling work, copying the timeout pointer, or claiming a catalog revision.
func NewConnectionToolSetRefreshResponse(target meta.ObjectReference, spec ConnectionToolSetRefreshSpec, status ConnectionToolSetActionStatus) ConnectionToolSetRefreshAction {
	return ConnectionToolSetRefreshAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionToolSetRefreshActionKind, target, spec, status,
	)}
}

// ValidateRequest checks the migration transport without resolving references,
// authorizing access, or deciding whether the requested generation is eligible.
func (a *ConnectionToolSetGenerationMigrationAction) ValidateRequest(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return validateConnectionToolSetActionSpec(a.Metadata.Target, a.Spec.ToolSetRef, a.Spec.TimeoutSeconds, true)
}

// ValidateResponse requires a valid migration spec and a tracking token. The
// eventual outcome belongs in task and binding status, not this acknowledgment.
func (a *ConnectionToolSetGenerationMigrationAction) ValidateResponse(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	if err := validateConnectionToolSetActionSpec(a.Metadata.Target, a.Spec.ToolSetRef, a.Spec.TimeoutSeconds, true); err != nil {
		return err
	}
	return validateConnectionToolSetActionStatus(a.Status)
}

// ValidateRequest checks a logical binding refresh request. MCP source checks,
// binding eligibility, URL-target agreement, and authorization are service work.
func (a *ConnectionToolSetRefreshAction) ValidateRequest(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return validateConnectionToolSetActionSpec(a.Metadata.Target, a.Spec.ToolSetRef, a.Spec.TimeoutSeconds, false)
}

// ValidateResponse requires a valid refresh spec and a tracking token. It cannot
// verify token authenticity or establish that a complete catalog was published.
func (a *ConnectionToolSetRefreshAction) ValidateResponse(expectedKind meta.Kind) error {
	if a == nil {
		return fmt.Errorf("$: action is required")
	}
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	if err := validateConnectionToolSetActionSpec(a.Metadata.Target, a.Spec.ToolSetRef, a.Spec.TimeoutSeconds, false); err != nil {
		return err
	}
	return validateConnectionToolSetActionStatus(a.Status)
}

// validateConnectionToolSetActionSpec shares the typed connection/ToolSet
// reference and timeout rules. Migration requires an exact generation; refresh
// forbids one. Name resolution, namespace ownership, and liveness are not shape
// checks and must be enforced when the service resolves the references.
func validateConnectionToolSetActionSpec(target, toolSetRef meta.ObjectReference, timeoutSeconds *int64, migration bool) error {
	if err := validateConnectionActionTarget(target); err != nil {
		return err
	}
	vc := &common.ValidationContext{Path: "$.spec.toolSetRef"}
	if err := meta.ValidateObjectReferenceWithOptions(toolSetRef, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       toolsetschema.ToolSetKind,
		IDValidator:        toolsetschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc); err != nil {
		return err
	}
	if migration && toolSetRef.Generation == 0 {
		return vc.NewErrorForField("generation", "must select a positive target generation")
	}
	if !migration && toolSetRef.Generation != 0 {
		return vc.NewErrorForField("generation", "must be omitted; refresh uses the binding's current generation")
	}
	if timeoutSeconds != nil && *timeoutSeconds <= 0 {
		return fmt.Errorf("$.spec.timeoutSeconds: must be greater than zero")
	}
	return nil
}

// validateConnectionToolSetActionStatus checks only the public acknowledgment
// shape. It deliberately does not interpret the opaque task tracking token.
func validateConnectionToolSetActionStatus(status *ConnectionToolSetActionStatus) error {
	if status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if strings.TrimSpace(status.TaskID) == "" {
		return fmt.Errorf("$.status.taskId: must not be blank")
	}
	return nil
}
