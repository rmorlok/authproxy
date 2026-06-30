package key

import (
	"encoding/json"
	"fmt"
)

func (kd *KeyData) MarshalJSON() ([]byte, error) {
	if kd == nil || kd.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(kd.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (kd *KeyData) UnmarshalJSON(data []byte) error {
	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var t KeyDataType

	if _, ok := valueMap["value"]; ok {
		t = &KeyDataValue{}
	} else if _, ok := valueMap["base64"]; ok {
		t = &KeyDataBase64Val{}
	} else if _, ok := valueMap["env_var"]; ok {
		t = &KeyDataEnvVar{}
	} else if _, ok := valueMap["env_var_base64"]; ok {
		t = &KeyDataEnvBase64Var{}
	} else if _, ok := valueMap["path"]; ok {
		t = &KeyDataFile{}
	} else if _, ok := valueMap["random"]; ok {
		t = &KeyDataRandomBytes{}
	} else if _, ok := valueMap["num_bytes"]; ok {
		t = &KeyDataRandomBytes{}
	} else if _, ok := valueMap["vault_transit_key_name"]; ok {
		t = &KeyDataVaultTransit{}
	} else if _, ok := valueMap["vault_address"]; ok {
		t = &KeyDataVault{}
	} else if _, ok := valueMap["aws_kms_key_id"]; ok {
		t = &KeyDataAwsKMS{}
	} else if _, ok := valueMap["aws_secret_id"]; ok {
		t = &KeyDataAwsSecret{}
	} else if _, ok := valueMap["gcp_kms_key_name"]; ok {
		t = &KeyDataGcpKMS{}
	} else if _, ok := valueMap["gcp_crypto_key"]; ok {
		t = &KeyDataGcpKMS{}
	} else if _, ok := valueMap["gcp_secret_name"]; ok {
		t = &KeyDataGcpSecret{}
	} else if _, ok := valueMap["mock_id"]; ok {
		t = &KeyDataMock{}
	} else if _, ok := valueMap["mock_kms_id"]; ok {
		t = &KeyDataMockKMS{}
	} else {
		return fmt.Errorf("invalid structure for value type; does not match value, base64, env_var, env_var_base64, path, random, num_bytes, vault_address, vault_transit_key_name, aws_kms_key_id, aws_secret_id, gcp_kms_key_name, gcp_secret_name, mock_id, mock_kms_id")
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	kd.InnerVal = t

	return nil
}
