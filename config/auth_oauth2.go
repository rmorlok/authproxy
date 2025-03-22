package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AuthOAuth2 struct {
	Type          AuthType                `json:"type" yaml:"type"`
	ClientId      StringValue             `json:"client_id" yaml:"client_id"`
	ClientSecret  StringValue             `json:"client_secret" yaml:"client_secret"`
	Scopes        []Scope                 `json:"scopes" yaml:"scopes"`
	Authorization AuthOauth2Authorization `json:"authorization" yaml:"authorization"`
	Token         AuthOauth2Token         `json:"token" yaml:"token"`
}

func (i *AuthOAuth2) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("auth oauth2 expected a mapping node, got %s", KindToString(value.Kind))
	}

	var clientIdSecret StringValue
	var clientSecretSecret StringValue

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "client_id":
			if clientIdSecret, err = stringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "client_secret":
			if clientSecretSecret, err = stringValueUnmarshalYAML(valueNode); err != nil {
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
