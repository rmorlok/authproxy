package config

import (
	"time"
)

type SystemAuth struct {
	JwtSigningKey       *Key                `json:"jwtSigningKey" yaml:"jwtSigningKey"`
	JwtIssuerVal        string              `json:"jwtIssuer" yaml:"jwtIssuer"`
	JwtTokenDurationVal time.Duration       `json:"jwtTokenDuration" yaml:"jwtTokenDuration"`
	DisableXSRF         bool                `json:"disableXsrf" yaml:"disableXsrf"`
	Actors              *ConfiguredActors   `json:"actors" yaml:"actors"`
	GlobalAESKey        *KeyData            `json:"globalAesKey" yaml:"globalAesKey"`
	DataEncryptionKeys  *DataEncryptionKeys `json:"dataEncryptionKeys,omitempty" yaml:"dataEncryptionKeys,omitempty"`
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
