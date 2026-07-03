package key

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ProviderType identifies the type of key data provider.
type ProviderType string

const (
	ProviderTypeValue                 ProviderType = "value"
	ProviderTypeBase64                ProviderType = "base64"
	ProviderTypeEnvVar                ProviderType = "env_var"
	ProviderTypeEnvVarBase64          ProviderType = "env_var_base64"
	ProviderTypeFile                  ProviderType = "file"
	ProviderTypeRandom                ProviderType = "random"
	ProviderTypeAwsSecretsManager     ProviderType = "aws_secrets_manager"
	ProviderTypeAwsKMS                ProviderType = "aws_kms"
	ProviderTypeGcp                   ProviderType = "gcp"
	ProviderTypeGcpKMS                ProviderType = "gcp_kms"
	ProviderTypeHashicorpVault        ProviderType = "hashicorpvault"
	ProviderTypeHashicorpVaultTransit ProviderType = "hashicorpvault_transit"
	ProviderTypeRaw                   ProviderType = "raw"
)

// KeyVersionProtectedData stores provider-specific wrapped key material for
// providers that protect a locally-generated DEK instead of returning key bytes.
type KeyVersionProtectedData struct {
	Type        string            `json:"type" yaml:"type"`
	WrappedData string            `json:"wrapped_data" yaml:"wrapped_data"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (p KeyVersionProtectedData) IsZero() bool {
	return p.Type == "" && p.WrappedData == "" && len(p.Metadata) == 0
}

func (p KeyVersionProtectedData) Value() (driver.Value, error) {
	if p.IsZero() {
		return nil, nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal KeyVersionProtectedData: %w", err)
	}
	return string(b), nil
}

func (p *KeyVersionProtectedData) Scan(value interface{}) error {
	if value == nil {
		*p = KeyVersionProtectedData{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case string:
		if v == "" {
			*p = KeyVersionProtectedData{}
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			*p = KeyVersionProtectedData{}
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into KeyVersionProtectedData", value)
	}

	return json.Unmarshal(data, p)
}

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
