package common

import (
	"encoding/json"
	"fmt"
)

func (sv *StringValue) MarshalJSON() ([]byte, error) {
	if sv.InnerVal == nil {
		return json.Marshal(nil)
	}

	// Direct value serialization is handled in the StringValueDirect implementation

	return json.Marshal(sv.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (sv *StringValue) UnmarshalJSON(data []byte) error {
	// Check for a direct string value
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		sv.InnerVal = &StringValueDirect{Value: string(data[1 : len(data)-1]), IsDirectString: true}
		return nil
	}

	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var keyData StringValueType

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
		return fmt.Errorf("invalid structure for value type; does not match value, base64, env_var, env_var_base64, path")
	}

	if err := json.Unmarshal(data, keyData); err != nil {
		return err
	}

	sv.InnerVal = keyData

	return nil
}
