package tools

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ToolKind identifies the canonical Tool resource in typed envelopes and references.
const ToolKind meta.Kind = "Tool"

// Tool is the canonical connection-bound resource for an authored operation.
// Its definition and metadata are editable for standalone Tools. Generated
// Tools carry server-owned ManagedBy status and can only be changed through
// their owning ToolSet. Revisions belong to status, not metadata.generation.
type Tool struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          ToolSpec        `json:"spec" yaml:"spec"`
	Status        *ToolStatus     `json:"status,omitempty" yaml:"status,omitempty"`
}

// ToolSpec combines a connection reference with the reusable definition while
// keeping definition fields flat in JSON and YAML. Connection references may
// change through standalone updates, but must resolve within the Tool's own
// namespace. The service resolves named references before persistence.
type ToolSpec struct {
	ConnectionRef  meta.ObjectReference `json:"connectionRef" yaml:"connectionRef"`
	ToolDefinition `json:",inline" yaml:",inline"`
}

// ToolStatus records the active definition revision and observed readiness.
// Revision is positive once stored and advances only when the service
// atomically publishes a replacement; applying a schema patch does not advance
// it. An empty Conditions list means readiness has not yet been observed.
type ToolStatus struct {
	Revision   uint64          `json:"revision" yaml:"revision"`
	Conditions []ToolCondition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	ManagedBy  *ToolManagedBy  `json:"managedBy,omitempty" yaml:"managedBy,omitempty"`
}

// ToolManagedBy identifies the ToolSet generation that controls a generated
// Tool. ToolSetRef carries its canonical immutable ID and applied generation.
// SourceKey is the exact source identity, independent of the Tool's display
// name; keys may contain operation paths or other provider-defined characters.
type ToolManagedBy struct {
	ToolSetRef meta.ObjectReference `json:"toolSetRef" yaml:"toolSetRef"`
	SourceKey  string               `json:"sourceKey" yaml:"sourceKey"`
}

// ToolCondition describes one observed aspect of a Tool revision. It uses the
// shared condition vocabulary but tracks revisions rather than generations.
// ObservedRevision zero means no revision has been observed; it may otherwise
// refer to the current or an older revision, never a future one.
type ToolCondition struct {
	Type               string               `json:"type" yaml:"type"`
	Status             meta.ConditionStatus `json:"status" yaml:"status"`
	ObservedRevision   uint64               `json:"observedRevision,omitempty" yaml:"observedRevision,omitempty"`
	LastTransitionTime time.Time            `json:"lastTransitionTime" yaml:"lastTransitionTime"`
	Reason             string               `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string               `json:"message,omitempty" yaml:"message,omitempty"`
}

// NewTool returns an empty Tool carrying its canonical API version and kind.
func NewTool() *Tool {
	return &Tool{TypeMeta: meta.NewTypeMeta(ToolKind)}
}

// NewToolReference returns a generation-free reference to an immutable Tool ID.
func NewToolReference(id apid.ID) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       ToolKind,
		ID:         id.String(),
	}
}

// GetId parses the Tool identity, returning apid.Nil when it is absent or invalid.
func (t *Tool) GetId() apid.ID {
	if t == nil || t.Metadata.ID == "" {
		return apid.Nil
	}

	if err := ValidateID(t.Metadata.ID); err != nil {
		return apid.Nil
	}

	id, _ := apid.Parse(t.Metadata.ID)

	return id
}

// ValidateID verifies that an identifier belongs to the Tool resource kind.
func ValidateID(value string) error {
	id, err := apid.Parse(value)

	if err != nil {
		return err
	}

	if id.Prefix() != apid.PrefixTool {
		return fmt.Errorf("must be a tool id")
	}

	return nil
}

// ApplyCreateDefaults returns a detached copy with a default name derived from
// the allocated ID. The caller still owns assigning the server ID and status;
// this helper does not publish a definition or allocate a revision.
func (t *Tool) ApplyCreateDefaults(id apid.ID) *Tool {
	clone := t.Clone()

	if clone != nil && clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}

	return clone
}

// Validate applies configuration-file rules to a Tool resource.
func (t *Tool) Validate(vc *common.ValidationContext) error {
	return t.ValidateFor(meta.ValidationModeConfig, vc)
}

// ValidateFor checks resource identity, desired state, and server-owned status
// for one lifecycle boundary. ID-only connection references still require the
// service to resolve and verify the actual target namespace before applying.
func (t *Tool) ValidateFor(
	mode meta.ValidationMode,
	vc *common.ValidationContext,
) error {
	vc = validationContext(vc)

	if t == nil {
		return vc.NewError("tool is required")
	}

	stored := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse

	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateResource(
		t.TypeMeta,
		t.Metadata,
		meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       ToolKind,
			RequireID:          stored,
			RequireName:        stored,
			RequireNamespace:   true,
			IDValidator:        ValidateID,
			NamespaceValidator: namespaceschema.ValidatePath,
		},
	))

	if t.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to tools"))
	}

	result = multierror.Append(result, t.Spec.ValidateForNamespace(t.Metadata.Namespace, vc.PushField("spec")))

	if stored && t.Spec.ConnectionRef.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("spec.connectionRef.id", "is required for a stored connection binding"))
	}

	result = multierror.Append(result, meta.ValidateStatus(t.Status, mode, vc))

	if stored && t.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}

	if t.Status != nil {
		result = multierror.Append(result, t.Status.ValidateForNamespace(t.Metadata.Namespace, vc.PushField("status")))
	}

	return result.ErrorOrNil()
}

// Validate checks a complete desired spec before its owning namespace is
// available. Declaring this method explicitly prevents the embedded definition's
// Validate method from silently skipping connection-reference validation.
func (s *ToolSpec) Validate(vc *common.ValidationContext) error {
	return s.ValidateForNamespace("", vc)
}

// ValidateForNamespace checks a complete desired spec, including the explicit
// namespace boundary on its connection reference. Reference resolution and
// execution compilation belong to the service, not these serialized contracts.
func (s *ToolSpec) ValidateForNamespace(namespace string, vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if s == nil {
		return vc.NewError("tool spec is required")
	}

	var result *multierror.Error

	result = multierror.Append(result, ValidateConnectionReference(s.ConnectionRef, namespace, vc.PushField("connectionRef")))

	result = multierror.Append(result, s.ToolDefinition.Validate(vc))

	return result.ErrorOrNil()
}

// ValidateConnectionReference enforces Connection reference shape without
// resolving it. A nonempty resourceNamespace requires any explicit reference
// namespace to match exactly; callers with an unresolved patch can pass an
// empty namespace and check the boundary after merging. Connections never carry
// a generation in Tool references.
func ValidateConnectionReference(ref meta.ObjectReference, resourceNamespace string, vc *common.ValidationContext) error {
	vc = validationContext(vc)

	var result *multierror.Error

	result = multierror.Append(result, meta.ValidateObjectReferenceWithOptions(
		ref,
		meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       connectionschema.ConnectionKind,
			IDValidator:        connectionschema.ValidateID,
			NamespaceValidator: namespaceschema.ValidatePath,
		},
		vc,
	))

	if ref.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "does not apply to connections"))
	}

	if resourceNamespace != "" &&
		ref.Namespace != "" &&
		ref.Namespace != resourceNamespace {
		result = multierror.Append(result, vc.NewErrorfForField("namespace", "must equal the tool namespace %q", resourceNamespace))
	}

	return result.ErrorOrNil()
}

// ValidateForNamespace checks observed revision, condition freshness, and
// generated ownership. This validates a snapshot; monotonic revision updates
// require an atomic comparison against the previously stored revision in core.
func (s *ToolStatus) ValidateForNamespace(namespace string, vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if s == nil {
		return vc.NewError("tool status is required")
	}

	var result *multierror.Error

	if s.Revision == 0 {
		result = multierror.Append(result, vc.NewErrorForField("revision", "must be greater than zero"))
	}

	types := map[string]bool{}

	for i, condition := range s.Conditions {
		path := vc.PushField("conditions").PushIndex(i)
		result = multierror.Append(result, meta.ValidateCondition(
			meta.Condition{
				Type:               condition.Type,
				Status:             condition.Status,
				LastTransitionTime: condition.LastTransitionTime,
				Reason:             condition.Reason,
				Message:            condition.Message,
			},
			path,
		))

		if types[condition.Type] {
			result = multierror.Append(result, path.NewErrorForField("type", "must be unique within tool conditions"))
		}

		types[condition.Type] = true

		if condition.ObservedRevision > s.Revision {
			result = multierror.Append(result, path.NewErrorForField("observedRevision", "must not exceed the active tool revision"))
		}
	}

	if s.ManagedBy != nil {
		result = multierror.Append(result, s.ManagedBy.ValidateForNamespace(namespace, vc.PushField("managedBy")))
	}

	return result.ErrorOrNil()
}

// ValidateForNamespace requires a canonical generation-specific ToolSet owner.
// If its namespace is supplied, the owner must be at or above the Tool. The
// service checks this boundary after resolving an ID-only owner reference.
func (m *ToolManagedBy) ValidateForNamespace(namespace string, vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if m == nil {
		return vc.NewError("tool owner is required")
	}

	var result *multierror.Error

	path := vc.PushField("toolSetRef")

	ref := m.ToolSetRef

	result = multierror.Append(result, meta.ValidateObjectReferenceWithOptions(
		ref,
		meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       "ToolSet",
			IDValidator:        meta.IdValidatorForPrefix(apid.PrefixToolSet),
			NamespaceValidator: namespaceschema.ValidatePath,
		},
		path,
	))

	if ref.ID == "" {
		result = multierror.Append(result, path.NewErrorForField("id", "is required for a stored ToolSet owner"))
	}

	if ref.Generation == 0 {
		result = multierror.Append(result, path.NewErrorForField("generation", "must identify the applied ToolSet generation"))
	}

	if namespace != "" && ref.Namespace != "" && !namespaceschema.IsSameOrChild(ref.Namespace, namespace) {
		result = multierror.Append(result, path.NewErrorForField("namespace", "must be the tool namespace or an ancestor"))
	}

	if strings.TrimSpace(m.SourceKey) == "" {
		result = multierror.Append(result, vc.NewErrorForField("sourceKey", "must not be empty"))
	}

	return result.ErrorOrNil()
}

// ValidateUpdate enforces invariants on a standalone update candidate. The
// candidate must retain existing server status; revision allocation and status
// transitions occur only in the publication transaction. Definition and
// reference validation must also be performed on the complete merged spec.
func ValidateUpdate(before, after *Tool, vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if before == nil || after == nil {
		return vc.NewError("before and after tools are required")
	}

	var result *multierror.Error

	if before.Status != nil && before.Status.ManagedBy != nil {
		result = multierror.Append(result, vc.NewErrorf("tool is managed by ToolSet %q; update the owning ToolSet instead", before.Status.ManagedBy.ToolSetRef.ID))
	}

	result = multierror.Append(result, meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc))
	result = multierror.Append(result, meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{ImmutableNamespace: true}, vc))

	if !reflect.DeepEqual(before.Status, after.Status) {
		result = multierror.Append(result, vc.NewErrorForField("status", "is server-owned and must remain unchanged"))
	}

	return result.ErrorOrNil()
}
