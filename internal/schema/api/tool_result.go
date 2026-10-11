package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ToolExecutionState describes what is known about upstream side effects when
// an invocation fails. A failure never implies that retrying is safe.
type ToolExecutionState string

const (
	ToolExecutionStateNotStarted      ToolExecutionState = "notStarted"
	ToolExecutionStateMayHaveExecuted ToolExecutionState = "mayHaveExecuted"
	ToolExecutionStateCompleted       ToolExecutionState = "completed"
)

// ToolResult is the transport-neutral outcome placed in an invocation action's
// status.result. It is an endpoint DTO, not a durable execution resource. The
// surrounding v1alpha1 API contract versions this envelope and its content.
type ToolResult struct {
	ToolID       string `json:"toolId" yaml:"toolId"`
	Revision     uint64 `json:"revision" yaml:"revision"`
	InvocationID string `json:"invocationId" yaml:"invocationId"`
	IsError      bool   `json:"isError" yaml:"isError"`

	// StructuredContent accepts any JSON value. Nil means absent; the bytes
	// "null" mean an explicitly supplied JSON null and must remain present.
	StructuredContent common.RawJSON `json:"structuredContent,omitempty" yaml:"structuredContent,omitempty"`

	// Content retains order and must be a nonnil array, even when empty.
	Content  []ToolContentBlock `json:"content" yaml:"content"`
	Metadata ToolResultMetadata `json:"metadata" yaml:"metadata"`
	Error    *ToolResultError   `json:"error,omitempty" yaml:"error,omitempty"`

	// Provider holds redacted upstream extension metadata as an opaque JSON
	// object. It cannot grant permission or supply AuthProxy instructions.
	Provider common.RawJSON `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// ToolResultMetadata identifies the captured connection and imported source
// revisions used for execution. Omitted generation/catalog fields do not apply
// to the source; a catalog revision always belongs to a ToolSet generation.
type ToolResultMetadata struct {
	ConnectionID      string  `json:"connectionId" yaml:"connectionId"`
	ToolSetGeneration *uint64 `json:"toolSetGeneration,omitempty" yaml:"toolSetGeneration,omitempty"`
	CatalogRevision   *uint64 `json:"catalogRevision,omitempty" yaml:"catalogRevision,omitempty"`
}

// ToolResultError is a safe public failure after invocation admission. Producers
// own redaction and platform-code policy; arbitrary exception stacks and raw
// upstream error bodies must not be copied into these fields automatically.
type ToolResultError struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`

	// Details is optional JSON, including explicit null. Its absence does not
	// imply that protected diagnostic information is unavailable in logs.
	Details common.RawJSON `json:"details,omitempty" yaml:"details,omitempty"`

	// Retryable is advice, never an instruction to replay an invocation.
	Retryable      bool               `json:"retryable" yaml:"retryable"`
	ExecutionState ToolExecutionState `json:"executionState" yaml:"executionState"`
}

// Validate checks a result's public shape and correlation identities. Runtime
// limits, redaction, schema checks, and execution policy remain producer duties.
func (r *ToolResult) Validate(vc *common.ValidationContext) error {
	vc = toolResultValidationContext(vc)

	if r == nil {
		return vc.NewError("tool result is required")
	}

	var result *multierror.Error

	if err := toolschema.ValidateID(r.ToolID); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("toolId", err.Error()))
	}

	if r.Revision == 0 {
		result = multierror.Append(result, vc.NewErrorForField("revision", "must be greater than zero"))
	}

	invocationID, err := apid.Parse(r.InvocationID)

	if err != nil || invocationID.Prefix() != apid.PrefixToolInvocation {
		result = multierror.Append(result, vc.NewErrorForField("invocationId", "must be a tool invocation id"))
	}

	if err := connectionschema.ValidateID(r.Metadata.ConnectionID); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata.connectionId", err.Error()))
	}

	if r.Metadata.ToolSetGeneration != nil && *r.Metadata.ToolSetGeneration == 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.toolSetGeneration", "must be greater than zero"))
	}

	if r.Metadata.CatalogRevision != nil {
		if *r.Metadata.CatalogRevision == 0 {
			result = multierror.Append(result, vc.NewErrorForField("metadata.catalogRevision", "must be greater than zero"))
		}

		if r.Metadata.ToolSetGeneration == nil {
			result = multierror.Append(result, vc.NewErrorForField("metadata.toolSetGeneration", "is required with catalogRevision"))
		}
	}

	if err := validateOptionalToolJSON(r.StructuredContent, false); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("structuredContent", err.Error()))
	}

	if err := validateOptionalToolJSON(r.Provider, true); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("provider", err.Error()))
	}

	if r.Content == nil {
		result = multierror.Append(result, vc.NewErrorForField("content", "must be an array, which may be empty"))
	}

	for i := range r.Content {
		result = multierror.Append(result, r.Content[i].Validate(vc.PushField("content").PushIndex(i)))
	}

	if r.IsError != (r.Error != nil) {
		result = multierror.Append(result, vc.NewErrorForField("error", "must be present exactly when isError is true"))
	}

	result = multierror.Append(result, r.Error.Validate(vc.PushField("error")))

	return result.ErrorOrNil()
}

// Validate checks an optional public error without deciding whether a code is
// reserved or whether an upstream operation should be retried.
func (e *ToolResultError) Validate(vc *common.ValidationContext) error {
	if e == nil {
		return nil
	}

	vc = toolResultValidationContext(vc)
	var result *multierror.Error

	if strings.TrimSpace(e.Code) == "" {
		result = multierror.Append(result, vc.NewErrorForField("code", "must not be blank"))
	}

	if strings.TrimSpace(e.Message) == "" {
		result = multierror.Append(result, vc.NewErrorForField("message", "must not be blank"))
	}

	if err := validateOptionalToolJSON(e.Details, false); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("details", err.Error()))
	}

	switch e.ExecutionState {
	case ToolExecutionStateNotStarted, ToolExecutionStateMayHaveExecuted, ToolExecutionStateCompleted:
	default:
		result = multierror.Append(result, vc.NewErrorForField("executionState", "must be notStarted, mayHaveExecuted, or completed"))
	}

	return result.ErrorOrNil()
}

// Clone detaches result content, raw JSON, metadata pointers, and error details.
// It preserves nil/empty distinctions and invalid values for later validation.
func (r *ToolResult) Clone() *ToolResult {
	if r == nil {
		return nil
	}

	clone := *r
	clone.StructuredContent = slices.Clone(r.StructuredContent)
	clone.Provider = slices.Clone(r.Provider)
	clone.Content = slices.Clone(r.Content)

	for i := range clone.Content {
		clone.Content[i] = *r.Content[i].Clone()
	}

	clone.Metadata.ToolSetGeneration = util.CloneValue(r.Metadata.ToolSetGeneration)
	clone.Metadata.CatalogRevision = util.CloneValue(r.Metadata.CatalogRevision)
	clone.Error = r.Error.Clone()

	return &clone
}

// Clone detaches arbitrary JSON error details without changing their meaning.
func (e *ToolResultError) Clone() *ToolResultError {
	if e == nil {
		return nil
	}

	clone := *e
	clone.Details = slices.Clone(e.Details)

	return &clone
}

// UnmarshalYAML preserves explicit null and numeric precision in structured
// content and nested JSON through the shared YAML-to-JSON conversion.
func (r *ToolResult) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON

	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	var decoded ToolResult

	if err := util.DecodeJSONStrict(raw, &decoded); err != nil {
		return err
	}

	*r = decoded

	return nil
}

// UnmarshalYAML preserves explicit null details when an error is decoded on its
// own as well as when it is nested within a ToolResult.
func (e *ToolResultError) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON

	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	var decoded ToolResultError

	if err := util.DecodeJSONStrict(raw, &decoded); err != nil {
		return err
	}

	*e = decoded

	return nil
}

// toolResultValidationContext supplies a root for standalone validation while
// retaining paths supplied by enclosing action or content validators.
func toolResultValidationContext(vc *common.ValidationContext) *common.ValidationContext {
	if vc == nil {
		return &common.ValidationContext{Path: "$"}
	}

	return vc
}

// validateOptionalToolJSON accepts absent bytes or one complete JSON value.
// Provider metadata additionally requires an object; explicit null is a value,
// not omission, and therefore does not satisfy that object requirement.
func validateOptionalToolJSON(raw common.RawJSON, objectOnly bool) error {
	if len(raw) == 0 {
		return nil
	}

	if !json.Valid(raw) {
		return fmt.Errorf("must contain one valid JSON value")
	}

	if objectOnly && bytes.TrimSpace(raw)[0] != '{' {
		return fmt.Errorf("must contain a JSON object")
	}

	return nil
}
