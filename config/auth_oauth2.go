package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AuthOAuth2 struct {
	Type                  AuthType `json:"type" yaml:"type"`
	ClientId              Secret   `json:"client_id" yaml:"client_id"`
	ClientSecret          Secret   `json:"client_secret" yaml:"client_secret"`
	Scopes                []Scope  `json:"scopes" yaml:"scopes"`
	AuthorizationEndpoint string   `json:"authorization_endpoint" yaml:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint" yaml:"token_endpoint"`
}

func (i *AuthOAuth2) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var clientIdSecret Secret
	var clientSecretSecret Secret

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "client_id":
			if clientIdSecret, err = secretUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "client_secret":
			if clientSecretSecret, err = secretUnmarshalYAML(valueNode); err != nil {
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
	type RawType AuthOAuth2
	raw := (*RawType)(i)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ClientId = clientIdSecret
	raw.ClientSecret = clientSecretSecret
	raw.Type = AuthTypeOAuth2

	return nil
}

func (a *AuthOAuth2) GetType() AuthType {
	return AuthTypeOAuth2
}
