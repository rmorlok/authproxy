package key

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
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
	case keys["envVar"]:
		keyData = &KeyDataEnvVar{}
	case keys["envVarBase64"]:
		keyData = &KeyDataEnvBase64Var{}
	case keys["path"]:
		keyData = &KeyDataFile{}
	case keys["random"] || keys["numBytes"]:
		keyData = &KeyDataRandomBytes{}
	case keys["vaultTransitKeyName"]:
		keyData = &KeyDataVaultTransit{}
	case keys["vaultAddress"]:
		keyData = &KeyDataVault{}
	case keys["awsKmsKeyId"]:
		keyData = &KeyDataAwsKMS{}
	case keys["awsSecretId"]:
		keyData = &KeyDataAwsSecret{}
	case keys["gcpKmsKeyName"] || keys["gcpCryptoKey"]:
		keyData = &KeyDataGcpKMS{}
	case keys["gcpSecretName"]:
		keyData = &KeyDataGcpSecret{}
	case keys["mockId"]:
		keyData = &KeyDataMock{}
	case keys["mockKmsId"]:
		keyData = &KeyDataMockKMS{}
	}

	if keyData == nil {
		return fmt.Errorf("invalid structure for key data type; does not match value, base64, envVar, envVarBase64, path, random, numBytes, vaultAddress, vaultTransitKeyName, awsKmsKeyId, awsSecretId, gcpKmsKeyName, gcpSecretName, mockId, mockKmsId")
	}

	if err := util.DecodeYAMLNodeStrict(value, keyData); err != nil {
		return err
	}

	kd.InnerVal = keyData

	return nil
}
