package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AuthType string

const (
	AuthTypeOAuth2 = AuthType("OAuth2")
	AuthTypeAPIKey = AuthType("api-key")
)

type Auth interface {
	GetType() AuthType
}

func UnmarshallYamlAuthString(data string) (Auth, error) {
	return UnmarshallYamlAuth([]byte(data))
}

func UnmarshallYamlAuth(data []byte) (Auth, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return authUnmarshalYAML(rootNode.Content[0])
}

// authUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func authUnmarshalYAML(value *yaml.Node) (Auth, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var auth Auth

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
			}
		}

	}

	if auth == nil {
		return nil, fmt.Errorf("invalid auth type must be: %s, %s", AuthTypeAPIKey, AuthTypeOAuth2)
	}

	if err := value.Decode(auth); err != nil {
		return nil, err
	}

	return auth, nil
}
