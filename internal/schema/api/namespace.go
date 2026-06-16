package api

import "time"

// NamespaceState is the lifecycle state of a namespace.
type NamespaceState string

const (
	NamespaceStateActive     NamespaceState = "active"
	NamespaceStateDestroying NamespaceState = "destroying"
	NamespaceStateDestroyed  NamespaceState = "destroyed"
)

// NamespaceJson represents a namespace returned by the API.
//
//	@Description	Namespace for organizing resources
type NamespaceJson struct {
	Path        string            `json:"path" yaml:"path" example:"root.acme"`
	State       NamespaceState    `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	KeyId       *string           `json:"key_id,omitempty" yaml:"key_id,omitempty" example:"key_test550e8400abcd"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// CreateNamespaceRequestJson represents a request to create a namespace.
//
//	@Description	Namespace creation request
type CreateNamespaceRequestJson struct {
	Path        string            `json:"path" yaml:"path" example:"root.acme"`
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

// UpdateNamespaceRequestJson represents a request to update namespace metadata.
//
//	@Description	Namespace update request
type UpdateNamespaceRequestJson struct {
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

// ListNamespacesResponseJson is a paginated namespace list response.
//
//	@Description	Paginated list of namespaces
type ListNamespacesResponseJson struct {
	Items  []NamespaceJson `json:"items" yaml:"items"`
	Cursor string          `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// SetNamespaceKeyRequestJson sets the key used by a namespace.
type SetNamespaceKeyRequestJson struct {
	KeyId string `json:"key_id" yaml:"key_id" example:"key_test550e8400abcd"`
}

// NamespaceKeyJson is the namespace key lookup response.
type NamespaceKeyJson struct {
	KeyId string `json:"key_id" yaml:"key_id" example:"key_test550e8400abcd"`
}
