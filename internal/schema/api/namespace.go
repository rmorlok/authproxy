package api

import (
	"time"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

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
	Path string `json:"path" yaml:"path" example:"root.acme"`
	// Name is automatically set to the final segment of Path and cannot be changed.
	Name        common.ResourceName `json:"name" yaml:"name" swaggertype:"string" example:"acme"`
	State       NamespaceState      `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	KeyId       *string             `json:"keyId,omitempty" yaml:"keyId,omitempty" example:"key_test550e8400abcd"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time           `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt" yaml:"updatedAt"`
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

// ListNamespacesResponseJson is the Kubernetes-style namespace list response.
type ListNamespacesResponseJson = apiv1alpha1.ResourceList[NamespaceJson]

func NewListNamespacesResponseJson(items []NamespaceJson, continueToken string) ListNamespacesResponseJson {
	return apiv1alpha1.NewResourceList("Namespace", items, apiv1alpha1.ListMeta{Continue: continueToken})
}

// SetNamespaceKeyRequestJson sets the key used by a namespace.
type SetNamespaceKeyRequestJson struct {
	KeyId string `json:"keyId" yaml:"keyId" example:"key_test550e8400abcd"`
}

// NamespaceKeyJson is the namespace key lookup response.
type NamespaceKeyJson struct {
	KeyId string `json:"keyId" yaml:"keyId" example:"key_test550e8400abcd"`
}
