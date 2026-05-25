package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
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
	Path            string            `json:"path" yaml:"path" example:"root.acme"`
	State           NamespaceState    `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	EncryptionKeyId *string           `json:"encryption_key_id,omitempty" yaml:"encryption_key_id,omitempty" example:"ek_test550e8400abcde"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt       time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at" yaml:"updated_at"`
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
	Items  []NamespaceJson `json:"items" yaml:"items" swaggertype:"array,object"`
	Cursor string          `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// SetNamespaceEncryptionKeyRequestJson sets the encryption key used by a namespace.
type SetNamespaceEncryptionKeyRequestJson struct {
	EncryptionKeyId string `json:"encryption_key_id" yaml:"encryption_key_id" example:"ek_test550e8400abcde"`
}

// NamespaceEncryptionKeyJson is the namespace encryption-key lookup response.
type NamespaceEncryptionKeyJson struct {
	EncryptionKeyId string `json:"encryption_key_id" yaml:"encryption_key_id" example:"ek_test550e8400abcde"`
}

// ActorJson represents an actor returned by the API.
//
//	@Description	Actor identity within a namespace
type ActorJson struct {
	Id          apid.ID           `json:"id" yaml:"id" swaggertype:"string" example:"act_test550e8400abcde"`
	Namespace   string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	ExternalId  string            `json:"external_id" yaml:"external_id" example:"user-123"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// CreateActorRequestJson represents a request to create an actor.
//
//	@Description	Actor creation request
type CreateActorRequestJson struct {
	ExternalId  string            `json:"external_id" yaml:"external_id" example:"user-123"`
	Namespace   string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateActorRequestJson represents a request to update actor metadata.
//
//	@Description	Actor update request
type UpdateActorRequestJson struct {
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

// ListActorsResponseJson is a paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	Items  []ActorJson `json:"items" yaml:"items" swaggertype:"array,object"`
	Cursor string      `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}
