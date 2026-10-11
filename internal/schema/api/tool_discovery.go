package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

const (
	ToolDescriptorKind  meta.Kind = "ToolDescriptor"
	AgentConnectionKind meta.Kind = "AgentConnection"
)

// ToolSchemaDetail identifies how much schema data discovery includes. Compact
// schemas retain validation constraints; they are not abbreviated previews.
type ToolSchemaDetail string

const (
	ToolSchemaDetailNone    ToolSchemaDetail = "none"
	ToolSchemaDetailCompact ToolSchemaDetail = "compact"
	ToolSchemaDetailFull    ToolSchemaDetail = "full"
)

// ToolReadinessState describes known invocation availability, independently of
// permission and connection probe health. Discovery only includes permitted tools.
type ToolReadinessState string

const (
	ToolReadinessStateReady       ToolReadinessState = "ready"
	ToolReadinessStateUnavailable ToolReadinessState = "unavailable"
	ToolReadinessStateUnknown     ToolReadinessState = "unknown"
)

// ToolReadiness is the server's current availability observation. Reason is an
// extensible recovery/diagnostic code; Message must be safe for the caller to see.
// A ready observation does not promise that a later invocation will succeed.
type ToolReadiness struct {
	State   ToolReadinessState `json:"state" yaml:"state"`
	Reason  string             `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message string             `json:"message,omitempty" yaml:"message,omitempty"`
}

// ToolHintProvenance identifies who made a behavioral claim. Provider claims are
// untrusted descriptive data, and neither origin changes permissions or retry policy.
type ToolHintProvenance string

const (
	ToolHintProvenanceAdministrator ToolHintProvenance = "administrator"
	ToolHintProvenanceProvider      ToolHintProvenance = "provider"
)

// ToolHint retains an explicit false claim and its individual provenance. An
// absent hint is represented by a nil pointer in ToolDescriptorHints.
type ToolHint struct {
	Value      bool               `json:"value" yaml:"value"`
	Provenance ToolHintProvenance `json:"provenance" yaml:"provenance"`
}

// ToolDescriptorHints permits mixed origins when an administrator overrides only
// some provider claims. These are presentation hints, never execution instructions.
type ToolDescriptorHints struct {
	ReadOnly    *ToolHint `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	Destructive *ToolHint `json:"destructive,omitempty" yaml:"destructive,omitempty"`
	Idempotent  *ToolHint `json:"idempotent,omitempty" yaml:"idempotent,omitempty"`
}

// ToolConnectionIdentity is the minimum connection identity exposed by a tool
// grant. It deliberately has no credentials, configuration, or arbitrary metadata.
type ToolConnectionIdentity struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name" yaml:"name"`
}

// ToolSetIdentity identifies the source generation installed for a generated
// tool. It is descriptive provenance and does not grant ToolSet management access.
type ToolSetIdentity struct {
	ID         string `json:"id" yaml:"id"`
	Name       string `json:"name" yaml:"name"`
	Generation uint64 `json:"generation" yaml:"generation"`
}

// ToolDescriptor is the safe usage projection returned by discovery and describe.
// It is separate from the management Tool resource and has no executable source,
// permission verbs, connection configuration, or arbitrary annotation maps.
// Producers authorize and redact before constructing it and bound all prose/data.
type ToolDescriptor struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	ToolID        string                 `json:"toolId" yaml:"toolId"`
	Revision      uint64                 `json:"revision" yaml:"revision"`
	Name          string                 `json:"name" yaml:"name"`
	Description   string                 `json:"description" yaml:"description"`
	Connection    ToolConnectionIdentity `json:"connection" yaml:"connection"`
	ToolSet       *ToolSetIdentity       `json:"toolSet,omitempty" yaml:"toolSet,omitempty"`

	// CatalogRevision applies only to a generated tool's connection-specific MCP
	// catalog. Omit it for standalone, explicit, and OpenAPI tools.
	CatalogRevision *uint64       `json:"catalogRevision,omitempty" yaml:"catalogRevision,omitempty"`
	Readiness       ToolReadiness `json:"readiness" yaml:"readiness"`

	// ConnectionHealth is an optional probe/credential observation, not an
	// availability decision. Unhealthy probes can coexist with a ready tool.
	ConnectionHealth *connectionschema.ConnectionHealthStatus `json:"connectionHealth,omitempty" yaml:"connectionHealth,omitempty"`
	Hints            *ToolDescriptorHints                     `json:"hints,omitempty" yaml:"hints,omitempty"`
	Schema           ToolSchemaDetail                         `json:"schema" yaml:"schema"`

	// InputSchema is required for compact/full detail. Producers compile schemas
	// before publication; these DTOs preserve their JSON without compiling again.
	InputSchema  common.RawJSON `json:"inputSchema,omitempty" yaml:"inputSchema,omitempty"`
	OutputSchema common.RawJSON `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty"`

	// Full detail may include bounded guidance and an array of argument objects.
	// Examples remain opaque JSON so nested nulls and large numbers survive.
	Usage    string         `json:"usage,omitempty" yaml:"usage,omitempty"`
	Examples common.RawJSON `json:"examples,omitempty" yaml:"examples,omitempty"`
}

// AgentConnectionSummary adds independently evaluated proxy capability to the
// minimal connection identity. A tool grant alone never implies CanProxy.
type AgentConnectionSummary struct {
	meta.TypeMeta          `json:",inline" yaml:",inline"`
	ToolConnectionIdentity `json:",inline" yaml:",inline"`
	CanProxy               bool `json:"canProxy" yaml:"canProxy"`
}

// ToolDescriptorList is one authorized discovery page. Its opaque continuation
// token must be bound to the principal, JWT scope, query, and catalog context.
type ToolDescriptorList struct {
	apiv1alpha1.ResourceList[ToolDescriptor] `json:",inline" yaml:",inline"`
}

// AgentConnectionList contains only connections visible through a permitted tool
// or a separate proxy grant; it is not the management connection inventory.
type AgentConnectionList struct {
	apiv1alpha1.ResourceList[AgentConnectionSummary] `json:",inline" yaml:",inline"`
}

// NewToolDescriptor initializes the version, omitted-schema detail, and unknown
// readiness. Callers must supply identity and description before validation.
func NewToolDescriptor() *ToolDescriptor {
	return &ToolDescriptor{
		TypeMeta:  meta.NewTypeMeta(ToolDescriptorKind),
		Schema:    ToolSchemaDetailNone,
		Readiness: ToolReadiness{State: ToolReadinessStateUnknown},
	}
}

// NewAgentConnectionSummary wraps an authorized identity and a separately
// evaluated proxy grant. It does not evaluate permissions or read connection data.
func NewAgentConnectionSummary(identity ToolConnectionIdentity, canProxy bool) AgentConnectionSummary {
	return AgentConnectionSummary{
		TypeMeta:               meta.NewTypeMeta(AgentConnectionKind),
		ToolConnectionIdentity: identity,
		CanProxy:               canProxy,
	}
}

// NewToolDescriptorList wraps a prefiltered page, normalizing nil items to [].
// It neither filters authorization nor copies the descriptors' mutable data.
func NewToolDescriptorList(items []ToolDescriptor, continueToken string) ToolDescriptorList {
	return ToolDescriptorList{ResourceList: apiv1alpha1.NewResourceList(
		ToolDescriptorKind, items, apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}

// NewAgentConnectionList wraps a prefiltered page, normalizing nil items to [].
func NewAgentConnectionList(items []AgentConnectionSummary, continueToken string) AgentConnectionList {
	return AgentConnectionList{ResourceList: apiv1alpha1.NewResourceList(
		AgentConnectionKind, items, apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}
