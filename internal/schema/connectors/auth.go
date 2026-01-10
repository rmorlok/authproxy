package connectors

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

type AuthType string

const (
	AuthTypeOAuth2 = AuthType("OAuth2")
	AuthTypeAPIKey = AuthType("api-key")
	AuthTypeNoAuth = AuthType("no-auth")
)

type AuthImpl interface {
	Clone() AuthImpl
	GetType() AuthType
}

type Auth struct {
	InnerVal AuthImpl `json:"-" yaml:"-"`
}

func (a *Auth) Inner() AuthImpl {
	return a.InnerVal
}

func (a *Auth) CloneValue() *Auth {
	if a.InnerVal == nil {
		return nil
	}

	return &Auth{InnerVal: a.InnerVal.Clone()}
}

func (a *Auth) Clone() AuthImpl {
	return a.CloneValue()
}

func (a *Auth) GetType() AuthType {
	return a.InnerVal.GetType()
}

func (a *Auth) MarshalYAML() (interface{}, error) {
	if a.InnerVal == nil {
		return nil, nil
	}

	// Delegate to the concrete type
	return a.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (a *Auth) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("auth expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var auth AuthImpl

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		if keyNode.Value == "type" {
			switch AuthType(valueNode.Value) {
			case AuthTypeOAuth2:
				auth = &AuthOAuth2{}
				break fieldLoop
			case AuthTypeAPIKey:
				auth = &AuthApiKey{}
				break fieldLoop
			case AuthTypeNoAuth:
				auth = &AuthNoAuth{}
				break fieldLoop
			}
		}

	}

	if auth == nil {
		return fmt.Errorf("invalid auth type must be: %s, %s, %s", AuthTypeAPIKey, AuthTypeOAuth2, AuthTypeNoAuth)
	}

	if err := value.Decode(auth); err != nil {
		return err
	}

	a.InnerVal = auth

	return nil
}

func (a *Auth) MarshalJSON() ([]byte, error) {
	if a.InnerVal == nil {
		return json.Marshal(nil)
	}

	// Direct value serialization is handled in the concrete type

	return json.Marshal(a.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (a *Auth) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var ai AuthImpl

	switch AuthType(m["type"].(string)) {
	case AuthTypeOAuth2:
		ai = &AuthOAuth2{}
	case AuthTypeAPIKey:
		ai = &AuthApiKey{}
	case AuthTypeNoAuth:
		ai = &AuthNoAuth{}
	}

	if ai == nil {
		return fmt.Errorf("invalid auth type '%s', possible types are: %s, %s, %s", m["type"], AuthTypeAPIKey, AuthTypeOAuth2, AuthTypeNoAuth)
	}

	if err := json.Unmarshal(data, ai); err != nil {
		return err
	}

	a.InnerVal = ai

	return nil
}

// NewNoAuth creates a new no-auth authenticator. Used to simplify testing.
func NewNoAuth() *Auth {
	return &Auth{
		InnerVal: &AuthNoAuth{
			Type: AuthTypeNoAuth,
		},
	}
}

var _ AuthImpl = (*Auth)(nil)
