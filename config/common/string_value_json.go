package common

import (
	"encoding/json"
	"fmt"
)

// StringValueUnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func StringValueUnmarshalJSON(data []byte) (StringValue, error) {
	// Try to unmarshal as a string first (for scalar values)
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		return &StringValueDirect{Value: stringValue}, nil
	}

	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var keyData StringValue

	if _, ok := valueMap["value"]; ok {
		keyData = &StringValueDirect{}
	} else if _, ok := valueMap["base64"]; ok {
		keyData = &StringValueBase64{}
	} else if _, ok := valueMap["env_var"]; ok {
		keyData = &StringValueEnvVar{}
	} else if _, ok := valueMap["env_var_base64"]; ok {
		keyData = &StringValueEnvVarBase64{}
	} else if _, ok := valueMap["path"]; ok {
		keyData = &StringValueFile{}
	} else {
		return nil, fmt.Errorf("invalid structure for value type; does not match value, base64, env_var, env_var_base64, path")
	}

	if err := json.Unmarshal(data, keyData); err != nil {
		return nil, err
	}

	return keyData, nil
}