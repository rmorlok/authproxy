package api

import (
	"time"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// ConnectionToolSetBindingKind identifies a read-only installation projection.
// Bindings remain controller-owned and do not have a public CRUD identity.
const ConnectionToolSetBindingKind meta.Kind = "ConnectionToolSetBinding"

// These condition types describe independent observations. Other condition types
// remain valid so new controller diagnostics do not require a contract revision.
const (
	ToolSetBindingConditionInstalled          = "Installed"
	ToolSetBindingConditionReady              = "Ready"
	ToolSetBindingConditionMigrationSucceeded = "MigrationSucceeded"
)

// ConnectionToolSetBinding reports one logical ToolSet's assignment to a
// connection. Metadata carries the connection namespace and optional observed
// metadata; spec references identify the pair, without a fabricated binding ID,
// name, or generation. All fields are server-produced, including spec.
type ConnectionToolSetBinding struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta                 `json:"metadata" yaml:"metadata"`
	Spec          ConnectionToolSetBindingSpec    `json:"spec" yaml:"spec"`
	Status        *ConnectionToolSetBindingStatus `json:"status" yaml:"status"`
}

// ConnectionToolSetBindingSpec identifies the assignment with canonical IDs.
// Both references are generation-free; the installed generation belongs in
// status and can be absent while waiting for primary publication or setup.
type ConnectionToolSetBindingSpec struct {
	ConnectionRef meta.ObjectReference `json:"connectionRef" yaml:"connectionRef"`
	ToolSetRef    meta.ObjectReference `json:"toolSetRef" yaml:"toolSetRef"`
}

// ConnectionToolSetBindingStatus describes the currently installed inventory,
// independently of an unsuccessful installation, migration, or refresh attempt.
type ConnectionToolSetBindingStatus struct {
	// AppliedGeneration is absent until an inventory has been activated. Failed
	// migrations retain the previously installed generation, even on rollback.
	AppliedGeneration *uint64 `json:"appliedGeneration,omitempty" yaml:"appliedGeneration,omitempty"`

	// Conditions use the relevant ToolSet generation as observedGeneration.
	// Installed/Ready describe the installation; MigrationSucceeded describes
	// the attempted target, which may be newer or older than AppliedGeneration.
	// Unknown is appropriate before an outcome is known. Omission makes no claim.
	Conditions []meta.Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	// Catalog is specific to this connection's MCP source. Explicit and OpenAPI
	// ToolSets omit it. Failed first discovery may report an attempt with no
	// lastComplete catalog or applied generation.
	Catalog *MCPCatalogStatus `json:"catalog,omitempty" yaml:"catalog,omitempty"`
}

// MCPCatalogFreshness records whether the retained complete enumeration is
// considered fresh. Freshness alone says nothing about current authentication
// context, parent lifecycle state, or permission to execute a tool.
type MCPCatalogFreshness string

const (
	MCPCatalogFreshnessFresh MCPCatalogFreshness = "fresh"
	MCPCatalogFreshnessStale MCPCatalogFreshness = "stale"
)

// MCPCatalogStatus separates the latest refresh attempt from the last complete
// enumeration. Failed or partial enumeration updates diagnostics without implying
// removal of previously observed tools. No endpoint, credentials, or raw catalog
// content is exposed by this management status.
type MCPCatalogStatus struct {
	// LastAttemptAt is the start time of the latest refresh attempt. It may
	// precede LastComplete.LastSuccessAt when that same attempt succeeded.
	LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty" yaml:"lastAttemptAt,omitempty"`

	// LastComplete remains visible after a refresh failure or context change.
	// Its presence does not imply that it is eligible for execution.
	LastComplete *MCPCatalogSnapshot `json:"lastComplete,omitempty" yaml:"lastComplete,omitempty"`

	// Diagnostics describes the latest attempt, not necessarily LastComplete.
	// Producers bound and redact these entries before exposing them to callers.
	Diagnostics []toolsetschema.SourceDiagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

// MCPCatalogSnapshot describes the last complete catalog and its current
// freshness/context observations. It belongs to the enclosing binding's applied
// generation. Internal epochs and context fingerprints remain private.
type MCPCatalogSnapshot struct {
	Revision      uint64              `json:"revision" yaml:"revision"`
	SourceDigest  string              `json:"sourceDigest" yaml:"sourceDigest"`
	LastSuccessAt time.Time           `json:"lastSuccessAt" yaml:"lastSuccessAt"`
	Freshness     MCPCatalogFreshness `json:"freshness" yaml:"freshness"`

	// ContextMatches is True only when the catalog context matches the current
	// connection context. False or Unknown makes the retained catalog unavailable.
	// A stale catalog may remain eligible only with True; admission rechecks the
	// live context rather than trusting this previously rendered observation.
	ContextMatches meta.ConditionStatus `json:"contextMatches" yaml:"contextMatches"`
}

// ConnectionToolSetBindingList is one management page of connection assignments.
// Callers must apply connection/ToolSet visibility checks before constructing it.
type ConnectionToolSetBindingList struct {
	apiv1alpha1.ResourceList[ConnectionToolSetBinding] `json:",inline" yaml:",inline"`
}

// NewConnectionToolSetBinding wraps a resolved pair with empty observed status.
// It does not resolve references, install inventory, or copy resource metadata.
func NewConnectionToolSetBinding(namespace string, connectionRef, toolSetRef meta.ObjectReference) *ConnectionToolSetBinding {
	return &ConnectionToolSetBinding{
		TypeMeta: meta.NewTypeMeta(ConnectionToolSetBindingKind),
		Metadata: meta.ObjectMeta{Namespace: namespace},
		Spec: ConnectionToolSetBindingSpec{
			ConnectionRef: connectionRef,
			ToolSetRef:    toolSetRef,
		},
		Status: &ConnectionToolSetBindingStatus{},
	}
}

// NewConnectionToolSetBindingList wraps an already authorized page, normalizing
// nil items to []. It neither copies nested status nor interprets the cursor.
func NewConnectionToolSetBindingList(items []ConnectionToolSetBinding, continueToken string) ConnectionToolSetBindingList {
	return ConnectionToolSetBindingList{ResourceList: apiv1alpha1.NewResourceList(
		ConnectionToolSetBindingKind, items, apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}
