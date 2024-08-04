package config

import (
	"encoding/base64"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type Secret interface {
	// GetValue returns the value and if it is present on the system
	GetValue() (secret string, present bool)
}

// secretUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func secretUnmarshalYAML(value *yaml.Node) (Secret, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var secret Secret

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			secret = &SecretValue{}
			break fieldLoop
		case "base64":
			secret = &SecretBase64Val{}
			break fieldLoop
		case "env_var":
			secret = &SecretEnvVar{}
			break fieldLoop
		case "path":
			secret = &SecretFile{}
			break fieldLoop
		}
	}

	if secret == nil {
		return nil, fmt.Errorf("invalid structure for secret type; does not match value, base64 or env_var")
	}

	if err := value.Decode(secret); err != nil {
		return nil, err
	}

	return secret, nil
}

type SecretValue struct {
	Value string `json:"value" yaml:"value"`
}

func (s *SecretValue) GetValue() (secret string, present bool) {
	return s.Value, true
}

type SecretBase64Val struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (s *SecretBase64Val) GetValue() (secret string, present bool) {
	decodedBytes, err := base64.StdEncoding.DecodeString(s.Base64)
	if err != nil {
		return "", false
	}

	return string(decodedBytes), true
}

type SecretEnvVar struct {
	EnvVar string `json:"env_var" yaml:"env_var"`
}

func (s *SecretEnvVar) GetValue() (secret string, present bool) {
	return os.LookupEnv(s.EnvVar)
}

type SecretFile struct {
	Path string `json:"path" yaml:"path"`
}

func (s *SecretFile) GetValue() (secret string, present bool) {
	if _, err := os.Stat(s.Path); os.IsNotExist(err) {
		return "", false
	}

	// Read the file contents
	content, err := os.ReadFile(s.Path)
	if err != nil {
		return "", false
	}

	val := strings.TrimSpace(string(content))
	return val, len(val) > 0
}
