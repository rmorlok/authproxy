package config

import (
	"crypto/sha256"
	"fmt"
)

// ProviderType identifies the type of key data provider.
type ProviderType string

const (
	ProviderTypeValue          ProviderType = "value"
	ProviderTypeBase64         ProviderType = "base64"
	ProviderTypeEnvVar         ProviderType = "env_var"
	ProviderTypeEnvVarBase64   ProviderType = "env_var_base64"
	ProviderTypeFile           ProviderType = "file"
	ProviderTypeRandom         ProviderType = "random"
	ProviderTypeAws            ProviderType = "aws"
	ProviderTypeGcp            ProviderType = "gcp"
	ProviderTypeHashicorpVault ProviderType = "hashicorpvault"
	ProviderTypeRaw            ProviderType = "raw"
)

// KeyVersionInfo contains metadata about a key version from a provider.
type KeyVersionInfo struct {
	Provider        ProviderType
	ProviderID      string
	ProviderVersion string
	Data            []byte
	IsCurrent       bool
}

// DataHash returns the SHA-256 hex digest of the given data.
func DataHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}
