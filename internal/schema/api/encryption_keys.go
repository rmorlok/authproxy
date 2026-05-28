package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	cfgschema "github.com/rmorlok/authproxy/internal/schema/config"
)

// EncryptionKeyState is the API-visible lifecycle state of an encryption key.
type EncryptionKeyState string

const (
	EncryptionKeyStateActive   EncryptionKeyState = "active"
	EncryptionKeyStateDisabled EncryptionKeyState = "disabled"
)

// EncryptionKeyJson is the API envelope for a managed encryption key.
//
//	@Description	Encryption key API response
type EncryptionKeyJson struct {
	Id          apid.ID            `json:"id" yaml:"id" swaggertype:"string" example:"ek_test550e8400abcde"`
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	State       EncryptionKeyState `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time          `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at" yaml:"updated_at"`
}

type ListEncryptionKeysResponseJson struct {
	Items  []EncryptionKeyJson `json:"items" yaml:"items"`
	Cursor string              `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// CreateEncryptionKeyRequestJson is the request body for POST /encryption-keys.
//
//	@Description	Request to create a new encryption key
type CreateEncryptionKeyRequestJson struct {
	Namespace   string             `json:"namespace" yaml:"namespace" example:"root.acme"`
	KeyData     *cfgschema.KeyData `json:"key_data,omitempty" yaml:"key_data,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateEncryptionKeyRequestJson is the request body for PATCH /encryption-keys/:id.
//
//	@Description	Request to update an encryption key
type UpdateEncryptionKeyRequestJson struct {
	State       *EncryptionKeyState `json:"state,omitempty" yaml:"state,omitempty"`
	Labels      *map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string  `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
