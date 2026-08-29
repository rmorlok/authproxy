package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
)

// KeyState is the API-visible lifecycle state of a key.
type KeyState string

const (
	KeyStateActive   KeyState = "active"
	KeyStateDisabled KeyState = "disabled"
)

// KeyJson is the API envelope for a managed key.
//
//	@Description	Key API response
type KeyJson struct {
	Id          apid.ID             `json:"id" yaml:"id" swaggertype:"string" example:"key_test550e8400abcd"`
	Namespace   string              `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName `json:"name" yaml:"name" swaggertype:"string" example:"primary-encryption-key"`
	State       KeyState            `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	KeyData     *keyschema.KeyData  `json:"keyData,omitempty" yaml:"keyData,omitempty"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time           `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt" yaml:"updatedAt"`
}

type ListKeysResponseJson = apiv1alpha1.ResourceList[KeyJson]

func NewListKeysResponseJson(items []KeyJson, continueToken string) ListKeysResponseJson {
	return apiv1alpha1.NewResourceList("Key", items, apiv1alpha1.ListMeta{Continue: continueToken})
}

// CreateKeyRequestJson is the request body for POST /keys.
//
//	@Description	Request to create a new key
type CreateKeyRequestJson struct {
	Namespace   string               `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"primary-encryption-key"`
	KeyData     *keyschema.KeyData   `json:"keyData,omitempty" yaml:"keyData,omitempty"`
	Labels      map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateKeyRequestJson is the request body for PATCH /keys/:id.
//
//	@Description	Request to update a key
type UpdateKeyRequestJson struct {
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"primary-encryption-key"`
	State       *KeyState            `json:"state,omitempty" yaml:"state,omitempty"`
	KeyData     *keyschema.KeyData   `json:"keyData,omitempty" yaml:"keyData,omitempty"`
	Labels      *map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
