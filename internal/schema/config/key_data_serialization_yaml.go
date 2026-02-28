package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (kd *KeyData) MarshalYAML() (interface{}, error) {
	if kd.InnerVal == nil {
		return nil, nil
	}

	return kd.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (kd *KeyData) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("key data expected a mapping node, got %s", KindToString(value.Kind))
	}

	var keyData KeyDataType

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
		case "env_var_base64":
			keyData = &KeyDataEnvBase64Var{}
			break fieldLoop
		case "path":
			keyData = &KeyDataFile{}
			break fieldLoop
		case "random":
			keyData = &KeyDataRandomBytes{}
			break fieldLoop
		case "vault_address":
			keyData = &KeyDataVault{}
			break fieldLoop
		case "aws_secret_id":
			keyData = &KeyDataAwsSecret{}
			break fieldLoop
		case "gcp_secret_name":
			keyData = &KeyDataGcpSecret{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return fmt.Errorf("invalid structure for key data type; does not match value, base64, env_var, env_var_base64, path, random, vault_address, aws_secret_id, gcp_secret_name")
	}

	if err := value.Decode(keyData); err != nil {
		return err
	}

	kd.InnerVal = keyData

	return nil
}
