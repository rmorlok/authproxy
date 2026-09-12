package client

import "time"

// APIVersion is the resource schema understood by this provider. The REST
// route remains /api/v1; apiVersion describes the JSON resource contract.
const APIVersion = "authproxy.net/v1alpha1"

// TypeMeta identifies the schema used to decode a resource.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// NewTypeMeta returns the canonical type metadata for a resource kind.
func NewTypeMeta(kind string) TypeMeta {
	return TypeMeta{APIVersion: APIVersion, Kind: kind}
}

// ObjectMetadata contains the identity and common metadata shared by
// AuthProxy resources. Pointer timestamps allow create requests to omit
// server-owned fields while using the same shape for responses.
type ObjectMetadata struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Generation  uint64            `json:"generation,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   *time.Time        `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time        `json:"updatedAt,omitempty"`
}

// ObjectMetadataPatch is the common metadata merge-patch shape. A non-nil
// pointer to an empty map clears that map.
type ObjectMetadataPatch struct {
	Name        *string            `json:"name,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// ObjectReference identifies another resource without copying its spec or
// status. Generation is only populated for generation-tracked resource kinds.
type ObjectReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Generation uint64 `json:"generation,omitempty"`
}

// NewIDReference creates a canonical ID-based object reference.
func NewIDReference(kind, id string) *ObjectReference {
	return &ObjectReference{APIVersion: APIVersion, Kind: kind, ID: id}
}

// ListMetadata contains resource-list pagination state.
type ListMetadata struct {
	ResourceVersion    string `json:"resourceVersion,omitempty"`
	Continue           string `json:"continue,omitempty"`
	RemainingItemCount *int64 `json:"remainingItemCount,omitempty"`
}

// ResourceList is the common Kubernetes-style list envelope.
type ResourceList[T any] struct {
	TypeMeta
	Metadata ListMetadata `json:"metadata"`
	Items    []T          `json:"items"`
}

// ActionMetadata identifies the resource targeted by an imperative API action.
type ActionMetadata struct {
	Target ObjectReference `json:"target"`
}

// ActionRequest is the Kubernetes-style envelope used by imperative API
// operations. Actions are transports rather than durable resources, but use
// the same type metadata and object-reference conventions.
type ActionRequest[TSpec any] struct {
	TypeMeta
	Metadata ActionMetadata `json:"metadata"`
	Spec     TSpec          `json:"spec"`
}

// NewActionRequest creates a canonical action request for target.
func NewActionRequest[TSpec any](kind string, target ObjectReference, spec TSpec) ActionRequest[TSpec] {
	return ActionRequest[TSpec]{
		TypeMeta: NewTypeMeta(kind),
		Metadata: ActionMetadata{Target: target},
		Spec:     spec,
	}
}
