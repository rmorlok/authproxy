package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"gopkg.in/yaml.v3"
)

type TlsConfig interface {
	TlsConfig(ctx context.Context) (*tls.Config, error)
}

func UnmarshallYamlTlsConfigString(data string) (TlsConfig, error) {
	return UnmarshallYamlTlsConfig([]byte(data))
}

func UnmarshallYamlTlsConfig(data []byte) (TlsConfig, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return tlsConfigUnmarshalYAML(rootNode.Content[0])
}

// tlsConfigUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func tlsConfigUnmarshalYAML(value *yaml.Node) (TlsConfig, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("tls config expected a mapping node, got %s", KindToString(value.Kind))
	}

	var tlsConfig TlsConfig

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "accept_tos":
			tlsConfig = &TlsConfigLetsEncrypt{}
			break fieldLoop
		case "auto_gen_path":
			tlsConfig = &TlsConfigSelfSignedAutogen{}
			break fieldLoop
		case "cert":
			tlsConfig = &TlsConfigVals{}
			break fieldLoop
		}
	}

	if tlsConfig == nil {
		return nil, fmt.Errorf("invalid structure for tls config type; does not match vals, lets encrypt, self-signed auto gen")
	}

	if err := value.Decode(tlsConfig); err != nil {
		return nil, err
	}

	return tlsConfig, nil
}
