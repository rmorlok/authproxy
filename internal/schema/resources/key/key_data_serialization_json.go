package key

import (
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/util"
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
	} else if _, ok := valueMap["envVar"]; ok {
		t = &KeyDataEnvVar{}
	} else if _, ok := valueMap["envVarBase64"]; ok {
		t = &KeyDataEnvBase64Var{}
	} else if _, ok := valueMap["path"]; ok {
		t = &KeyDataFile{}
	} else if _, ok := valueMap["random"]; ok {
		t = &KeyDataRandomBytes{}
	} else if _, ok := valueMap["numBytes"]; ok {
		t = &KeyDataRandomBytes{}
	} else if _, ok := valueMap["vaultTransitKeyName"]; ok {
		t = &KeyDataVaultTransit{}
	} else if _, ok := valueMap["vaultAddress"]; ok {
		t = &KeyDataVault{}
	} else if _, ok := valueMap["awsKmsKeyId"]; ok {
		t = &KeyDataAwsKMS{}
	} else if _, ok := valueMap["awsSecretId"]; ok {
		t = &KeyDataAwsSecret{}
	} else if _, ok := valueMap["gcpKmsKeyName"]; ok {
		t = &KeyDataGcpKMS{}
	} else if _, ok := valueMap["gcpCryptoKey"]; ok {
		t = &KeyDataGcpKMS{}
	} else if _, ok := valueMap["gcpSecretName"]; ok {
		t = &KeyDataGcpSecret{}
	} else if _, ok := valueMap["mockId"]; ok {
		t = &KeyDataMock{}
	} else if _, ok := valueMap["mockKmsId"]; ok {
		t = &KeyDataMockKMS{}
	} else {
		return fmt.Errorf("invalid structure for value type; does not match value, base64, envVar, envVarBase64, path, random, numBytes, vaultAddress, vaultTransitKeyName, awsKmsKeyId, awsSecretId, gcpKmsKeyName, gcpSecretName, mockId, mockKmsId")
	}

	if err := util.DecodeJSONStrict(data, t); err != nil {
		return err
	}

	kd.InnerVal = t

	return nil
}
