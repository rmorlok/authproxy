package api

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// Validate checks a management projection without authorizing access or deciding
// whether a binding can execute. Reference resolution, parent lifecycle checks,
// inventory activation, and stale-job fencing belong to the service.
func (b *ConnectionToolSetBinding) Validate(vc *common.ValidationContext) error {
	vc = bindingValidationContext(vc)

	if b == nil {
		return vc.NewError("connection ToolSet binding is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateResource(b.TypeMeta, b.Metadata, meta.ValidationOptions{
		Mode:               meta.ValidationModeResponse,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       ConnectionToolSetBindingKind,
		RequireNamespace:   true,
		NamespaceValidator: namespaceschema.ValidatePath,
	}))

	if b.Metadata.ID != "" || b.Metadata.Name != "" || b.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "binding projections do not have an id, name, or generation; spec references identify the pair"))
	}

	connectionPath := vc.PushField("spec").PushField("connectionRef")
	result = multierror.Append(result, toolschema.ValidateConnectionReference(b.Spec.ConnectionRef, "", connectionPath))

	if b.Spec.ConnectionRef.ID == "" {
		result = multierror.Append(result, connectionPath.NewErrorForField("id", "is required for a resolved connection"))
	}

	if b.Metadata.Namespace != "" && b.Spec.ConnectionRef.Namespace != "" && b.Spec.ConnectionRef.Namespace != b.Metadata.Namespace {
		result = multierror.Append(result, connectionPath.NewErrorForField("namespace", "must equal metadata.namespace"))
	}

	result = multierror.Append(result, validateBindingToolSetReference(b.Spec.ToolSetRef, b.Metadata.Namespace, vc.PushField("spec").PushField("toolSetRef")))

	if b.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
		return result.ErrorOrNil()
	}

	statusPath := vc.PushField("status")

	if b.Status.AppliedGeneration != nil && *b.Status.AppliedGeneration == 0 {
		result = multierror.Append(result, statusPath.NewErrorForField("appliedGeneration", "must be greater than zero when installed"))
	}

	conditionTypes := make(map[string]bool, len(b.Status.Conditions))

	for i, condition := range b.Status.Conditions {
		path := statusPath.PushField("conditions").PushIndex(i)
		result = multierror.Append(result, meta.ValidateCondition(condition, path))

		if conditionTypes[condition.Type] {
			result = multierror.Append(result, path.NewErrorForField("type", "must be unique within binding conditions"))
		}

		conditionTypes[condition.Type] = true
	}

	result = multierror.Append(result, b.Status.Catalog.Validate(statusPath.PushField("catalog")))

	if b.Status.Catalog != nil && b.Status.Catalog.LastComplete != nil && b.Status.AppliedGeneration == nil {
		result = multierror.Append(result, statusPath.NewErrorForField("appliedGeneration", "is required for the retained catalog's ToolSet generation"))
	}

	return result.ErrorOrNil()
}

// validateBindingToolSetReference requires a logical ToolSet ID and, when its
// namespace is supplied, an owner at or above the connection namespace. The
// service must resolve an ID-only reference before trusting that relationship.
func validateBindingToolSetReference(ref meta.ObjectReference, namespace string, vc *common.ValidationContext) error {
	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateObjectReferenceWithOptions(ref, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       toolsetschema.ToolSetKind,
		IDValidator:        toolsetschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc))

	if ref.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("id", "is required for a resolved ToolSet"))
	}

	if ref.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "spec identifies the logical ToolSet; the installed generation belongs in status.appliedGeneration"))
	}

	if namespace != "" && ref.Namespace != "" && !namespaceschema.IsSameOrChild(ref.Namespace, namespace) {
		result = multierror.Append(result, vc.NewErrorForField("namespace", "must be the connection namespace or an ancestor"))
	}

	return result.ErrorOrNil()
}

// Validate checks an optional catalog report. A completed catalog requires its
// attempt timestamp, but timestamps are not ordered: one attempt can start before
// its successful completion. These checks neither fetch nor erase catalog data.
func (c *MCPCatalogStatus) Validate(vc *common.ValidationContext) error {
	if c == nil {
		return nil
	}

	vc = bindingValidationContext(vc)
	var result *multierror.Error

	if c.LastAttemptAt != nil && c.LastAttemptAt.IsZero() {
		result = multierror.Append(result, vc.NewErrorForField("lastAttemptAt", "must not be zero when supplied"))
	}

	if c.LastComplete != nil {
		result = multierror.Append(result, c.LastComplete.Validate(vc.PushField("lastComplete")))

		if c.LastAttemptAt == nil {
			result = multierror.Append(result, vc.NewErrorForField("lastAttemptAt", "is required with a completed catalog"))
		}
	}

	for i := range c.Diagnostics {
		result = multierror.Append(result, c.Diagnostics[i].Validate(vc.PushField("diagnostics").PushIndex(i)))
	}

	return result.ErrorOrNil()
}

// Validate checks one retained enumeration's identity and observations. A stale
// or context-mismatched snapshot is valid status data, not an execution grant.
func (s *MCPCatalogSnapshot) Validate(vc *common.ValidationContext) error {
	vc = bindingValidationContext(vc)

	if s == nil {
		return vc.NewError("catalog snapshot is required")
	}

	var result *multierror.Error

	if s.Revision == 0 {
		result = multierror.Append(result, vc.NewErrorForField("revision", "must be greater than zero"))
	}

	if strings.TrimSpace(s.SourceDigest) == "" {
		result = multierror.Append(result, vc.NewErrorForField("sourceDigest", "must not be blank"))
	}

	if s.LastSuccessAt.IsZero() {
		result = multierror.Append(result, vc.NewErrorForField("lastSuccessAt", "is required"))
	}

	switch s.Freshness {
	case MCPCatalogFreshnessFresh, MCPCatalogFreshnessStale:
	default:
		result = multierror.Append(result, vc.NewErrorForField("freshness", "must be fresh or stale"))
	}

	switch s.ContextMatches {
	case meta.ConditionTrue, meta.ConditionFalse, meta.ConditionUnknown:
	default:
		result = multierror.Append(result, vc.NewErrorForField("contextMatches", "must be True, False, or Unknown"))
	}

	return result.ErrorOrNil()
}

// Validate checks pagination metadata and each binding projection in the page.
// It does not establish that all entries belong to the connection in a route URL.
func (l *ConnectionToolSetBindingList) Validate() error {
	if l == nil {
		return fmt.Errorf("$: connection ToolSet binding list is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, l.ResourceList.Validate(ConnectionToolSetBindingKind))

	if l.Items == nil {
		result = multierror.Append(result, fmt.Errorf("$.items: must be an array"))
	}

	for i := range l.Items {
		result = multierror.Append(result, l.Items[i].Validate((&common.ValidationContext{Path: "$.items"}).PushIndex(i)))
	}

	return result.ErrorOrNil()
}

// bindingValidationContext supplies the root path for standalone status checks.
func bindingValidationContext(vc *common.ValidationContext) *common.ValidationContext {
	if vc == nil {
		return &common.ValidationContext{Path: "$"}
	}

	return vc
}
