package config

import (
	"encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/yaml.v3"
	"os"
)

type KeyData interface {
	// HasData checks if this value has data.
	HasData(ctx context.Context) bool

	// GetData retrieves the bytes of the key
	GetData(ctx context.Context) ([]byte, error)
}

func UnmarshallYamlKeyDataString(data string) (Auth, error) {
	return UnmarshallYamlKeyData([]byte(data))
}

func UnmarshallYamlKeyData(data []byte) (Auth, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return authUnmarshalYAML(rootNode.Content[0])
}

// keyUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func keyDataUnmarshalYAML(value *yaml.Node) (KeyData, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var keyData KeyData

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			keyData = &KeyDataValue{}
			break fieldLoop
		case "base64":
			keyData = &KeyDataBase64Val{}
			break fieldLoop
		case "env_var":
			keyData = &KeyDataEnvVar{}
			break fieldLoop
		case "path":
			keyData = &KeyDataFile{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return nil, fmt.Errorf("invalid structure for key data type; does not match value, value, base64, env_var, file")
	}

	if err := value.Decode(keyData); err != nil {
		return nil, err
	}

	return keyData, nil
}

type KeyDataValue struct {
	Value string `json:"value" yaml:"value"`
}

func (kv *KeyDataValue) HasData(ctx context.Context) bool {
	return len(kv.Value) > 0
}

func (kv *KeyDataValue) GetData(ctx context.Context) ([]byte, error) {
	return []byte(kv.Value), nil
}

type KeyDataBase64Val struct {
	Base64 string `json:"base64" yaml:"base64"`
}

func (kb *KeyDataBase64Val) HasData(ctx context.Context) bool {
	return len(kb.Base64) > 0
}

func (kb *KeyDataBase64Val) GetData(ctx context.Context) ([]byte, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(kb.Base64)
	if err != nil {
		return nil, err
	}

	return decodedBytes, nil
}

type KeyDataEnvVar struct {
	EnvVar string `json:"env_var" yaml:"env_var"`
}

func (kev *KeyDataEnvVar) HasData(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *KeyDataEnvVar) GetData(ctx context.Context) ([]byte, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return nil, errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}
	return []byte(val), nil
}

type KeyDataFile struct {
	Path string `json:"path" yaml:"path"`
}

func (kf *KeyDataFile) HasData(ctx context.Context) bool {
	if _, err := os.Stat(kf.Path); os.IsNotExist(err) {
		return false
	}

	return true
}

func (kf *KeyDataFile) GetData(ctx context.Context) ([]byte, error) {
	if _, err := os.Stat(kf.Path); os.IsNotExist(err) {
		return nil, errors.Errorf("key file '%s' does not exist", kf.Path)
	}

	// Read the file contents
	return os.ReadFile(kf.Path)
}
