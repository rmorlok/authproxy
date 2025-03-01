package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"time"
)

type SystemAuth struct {
	JwtSigningKey       Key           `json:"jwt_signing_key" yaml:"jwt_signing_key"`
	JwtIssuerVal        string        `json:"jwt_issuer" yaml:"jwt_issuer"`
	JwtTokenDurationVal time.Duration `json:"jwt_token_duration" yaml:"jwt_token_duration"`
	DisableXSRF         bool          `json:"disable_xsrf" yaml:"disable_xsrf"`
	AdminUsers          AdminUsers    `json:"admin_users" yaml:"admin_users"`
	AdminEmailDomain    string        `json:"admin_email_domain,omitempty" yaml:"admin_email_domain,omitempty"`
	GlobalAESKey        KeyData       `json:"global_aes_key" yaml:"global_aes_key"`
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

func (sa *SystemAuth) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("system auth expected a mapping node, got %s", KindToString(value.Kind))
	}

	var jwtSigngingKey Key
	var adminUsers AdminUsers
	var globalAESKey KeyData

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "jwt_signing_key":
			if jwtSigngingKey, err = keyUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "admin_users":
			if adminUsers, err = adminUsersUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "global_aes_key":
			if globalAESKey, err = keyDataUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType SystemAuth
	raw := (*RawType)(sa)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.JwtSigningKey = jwtSigngingKey
	raw.AdminUsers = adminUsers
	raw.GlobalAESKey = globalAESKey

	return nil
}

func UnmarshallYamlSystemAuthString(data string) (*SystemAuth, error) {
	return UnmarshallYamlSystemAuth([]byte(data))
}

func UnmarshallYamlSystemAuth(data []byte) (*SystemAuth, error) {
	var systemAuth SystemAuth
	if err := yaml.Unmarshal(data, &systemAuth); err != nil {
		return nil, err
	}

	return &systemAuth, nil
}
