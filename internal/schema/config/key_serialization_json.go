package config

import (
	"encoding/json"
	"fmt"
)

func (k *Key) MarshalJSON() ([]byte, error) {
	if k == nil || k.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(k.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (k *Key) UnmarshalJSON(data []byte) error {
	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal key value: %v", err)
	}

	var t KeyType

	if _, ok := valueMap["public_key"]; ok {
		t = &KeyPublicPrivate{}
	} else if _, ok := valueMap["private_key"]; ok {
		t = &KeyPublicPrivate{}
	} else if _, ok := valueMap["shared_key"]; ok {
		t = &KeyShared{}
	} else {
		return fmt.Errorf("invalid structure for key type; does not match public_key, private_key or shared_key")
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	k.InnerVal = t

	return nil
}
