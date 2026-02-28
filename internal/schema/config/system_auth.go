package config

import (
	"fmt"
	"time"
)

type SystemAuth struct {
	JwtSigningKey       *Key              `json:"jwt_signing_key" yaml:"jwt_signing_key"`
	JwtIssuerVal        string            `json:"jwt_issuer" yaml:"jwt_issuer"`
	JwtTokenDurationVal time.Duration     `json:"jwt_token_duration" yaml:"jwt_token_duration"`
	DisableXSRF         bool              `json:"disable_xsrf" yaml:"disable_xsrf"`
	Actors              *ConfiguredActors `json:"actors" yaml:"actors"`
	GlobalAESKey        *KeyData          `json:"global_aes_key" yaml:"global_aes_key"`
	GlobalAESKeys       []*KeyData        `json:"global_aes_keys" yaml:"global_aes_keys"`
}

// ValidateGlobalAESKeys checks that only one of GlobalAESKey or GlobalAESKeys is set.
func (sa *SystemAuth) ValidateGlobalAESKeys() error {
	hasKey := sa.GlobalAESKey != nil
	hasKeys := len(sa.GlobalAESKeys) > 0

	if hasKey && hasKeys {
		return fmt.Errorf("only one of global_aes_key or global_aes_keys may be set, not both")
	}

	return nil
}

// GetGlobalAESKeys normalizes both GlobalAESKey and GlobalAESKeys into a single list.
// The primary key (used for encryption) is always the first element.
func (sa *SystemAuth) GetGlobalAESKeys() []*KeyData {
	if len(sa.GlobalAESKeys) > 0 {
		return sa.GlobalAESKeys
	}

	if sa.GlobalAESKey != nil {
		return []*KeyData{sa.GlobalAESKey}
	}

	return nil
}

func (sa *SystemAuth) JwtIssuer() string {
	if sa.JwtIssuerVal == "" {
		return "auth-proxy"
	}

	return sa.JwtIssuerVal
}

func (sa *SystemAuth) JwtTokenDuration() time.Duration {
	if sa.JwtTokenDurationVal == 0 {
		return 1 * time.Hour
	}

	return sa.JwtTokenDurationVal
}
