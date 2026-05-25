package connectors

// PKCEMethod selects the code-challenge transformation used by the
// connector's authorize/token-exchange pair. Values follow RFC 7636 §4.2.
type PKCEMethod string

const (
	// PKCEMethodS256 is the recommended challenge method:
	// challenge = base64url(sha256(verifier)). Always use this unless the
	// 3rd-party explicitly rejects it.
	PKCEMethodS256 = PKCEMethod("S256")
	// PKCEMethodPlain is the fallback method: challenge = verifier. RFC 7636
	// §4.2 marks plain as discouraged — it removes the cryptographic
	// hop that protects the verifier on a leaky TLS terminator. Configure
	// only when a provider cannot validate S256.
	PKCEMethodPlain = PKCEMethod("plain")
)

// AuthOauth2PKCE turns on RFC 7636 Proof Key for Code Exchange for a
// connector's authorization-code flow. nil disables PKCE (current default
// behavior); a non-nil block enables it with the chosen method.
type AuthOauth2PKCE struct {
	// Method is the challenge transformation. Defaults to S256 when unset.
	Method PKCEMethod `json:"method,omitempty" yaml:"method,omitempty"`
}

// GetMethodOrDefault returns the configured method, defaulting to S256 if
// the block is present but the method was omitted.
func (p *AuthOauth2PKCE) GetMethodOrDefault() PKCEMethod {
	if p == nil || p.Method == "" {
		return PKCEMethodS256
	}
	return p.Method
}

func (p *AuthOauth2PKCE) Clone() *AuthOauth2PKCE {
	if p == nil {
		return nil
	}
	clone := *p
	return &clone
}

type AuthOauth2Authorization struct {
	Endpoint       string            `json:"endpoint" yaml:"endpoint"`
	QueryOverrides map[string]string `json:"query_overrides,omitempty" yaml:"query_overrides,omitempty"`
	PKCE           *AuthOauth2PKCE   `json:"pkce,omitempty" yaml:"pkce,omitempty"`
}
