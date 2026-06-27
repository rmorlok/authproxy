package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
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
	Id          apid.ID            `json:"id" yaml:"id" swaggertype:"string" example:"key_test550e8400abcd"`
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	State       KeyState           `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty" yaml:"key_data,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time          `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at" yaml:"updated_at"`
}

type ListKeysResponseJson struct {
	Items  []KeyJson `json:"items" yaml:"items"`
	Cursor string    `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// CreateKeyRequestJson is the request body for POST /keys.
//
//	@Description	Request to create a new key
type CreateKeyRequestJson struct {
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty" yaml:"key_data,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateKeyRequestJson is the request body for PATCH /keys/:id.
//
//	@Description	Request to update a key
type UpdateKeyRequestJson struct {
	State       *KeyState          `json:"state,omitempty" yaml:"state,omitempty"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty" yaml:"key_data,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
