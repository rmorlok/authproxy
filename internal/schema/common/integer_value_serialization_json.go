package common

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func (sv *IntegerValue) MarshalJSON() ([]byte, error) {
	if sv.InnerVal == nil {
		return json.Marshal(nil)
	}

	// Direct value serialization is handled in the IntegerValueDirect implementation

	return json.Marshal(sv.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (sv *IntegerValue) UnmarshalJSON(data []byte) error {
	// Check for a direct integer value
	if val, err := strconv.ParseInt(string(data), 10, 64); err == nil {
		sv.InnerVal = &IntegerValueDirect{Value: val, IsDirect: true}
		return nil
	}

	// If it's not an integer, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal integer value: %v", err)
	}

	var svi IntegerValueType

	if _, ok := valueMap["value"]; ok {
		svi = &IntegerValueDirect{}
	} else if _, ok := valueMap["env_var"]; ok {
		svi = &IntegerValueEnvVar{}
	} else {
		return fmt.Errorf("invalid structure for value type; does not match direct value, value attribute, or env_var")
	}

	if err := json.Unmarshal(data, svi); err != nil {
		return err
	}

	sv.InnerVal = svi

	return nil
}
