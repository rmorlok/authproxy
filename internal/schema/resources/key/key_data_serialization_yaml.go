package key

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
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
		return fmt.Errorf("key data expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var keyData KeyDataType
	keys := map[string]bool{}

	for i := 0; i < len(value.Content); i += 2 {
		keys[value.Content[i].Value] = true
	}

	switch {
	case keys["value"]:
		keyData = &KeyDataValue{}
	case keys["base64"]:
		keyData = &KeyDataBase64Val{}
	case keys["env_var"]:
		keyData = &KeyDataEnvVar{}
	case keys["env_var_base64"]:
		keyData = &KeyDataEnvBase64Var{}
	case keys["path"]:
		keyData = &KeyDataFile{}
	case keys["random"] || keys["num_bytes"]:
		keyData = &KeyDataRandomBytes{}
	case keys["num_bytes"]:
		keyData = &KeyDataRandomBytes{}
	case keys["vault_transit_key_name"]:
		keyData = &KeyDataVaultTransit{}
	case keys["vault_address"]:
		keyData = &KeyDataVault{}
	case keys["aws_kms_key_id"]:
		keyData = &KeyDataAwsKMS{}
	case keys["aws_secret_id"]:
		keyData = &KeyDataAwsSecret{}
	case keys["gcp_kms_key_name"] || keys["gcp_crypto_key"]:
		keyData = &KeyDataGcpKMS{}
	case keys["gcp_secret_name"]:
		keyData = &KeyDataGcpSecret{}
	case keys["mock_id"]:
		keyData = &KeyDataMock{}
	case keys["mock_kms_id"]:
		keyData = &KeyDataMockKMS{}
	}

	if keyData == nil {
		return fmt.Errorf("invalid structure for key data type; does not match value, base64, env_var, env_var_base64, path, random, num_bytes, vault_address, vault_transit_key_name, aws_kms_key_id, aws_secret_id, gcp_kms_key_name, gcp_secret_name, mock_id, mock_kms_id")
	}

	if err := value.Decode(keyData); err != nil {
		return err
	}

	kd.InnerVal = keyData

	return nil
}
