package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type SystemAuth struct {
	JwtSigningKey       *Key          `json:"jwt_signing_key" yaml:"jwt_signing_key"`
	JwtIssuerVal        string        `json:"jwt_issuer" yaml:"jwt_issuer"`
	JwtTokenDurationVal time.Duration `json:"jwt_token_duration" yaml:"jwt_token_duration"`
	DisableXSRF         bool          `json:"disable_xsrf" yaml:"disable_xsrf"`
	AdminUsers          *AdminUsers   `json:"admin_users" yaml:"admin_users"`
	AdminEmailDomain    string        `json:"admin_email_domain,omitempty" yaml:"admin_email_domain,omitempty"`
	GlobalAESKey        *KeyData      `json:"global_aes_key" yaml:"global_aes_key"`
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
