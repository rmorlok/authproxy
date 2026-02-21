package config

import (
	"encoding/json"
	"fmt"
)

func (r *Redis) MarshalJSON() ([]byte, error) {
	if r == nil || r.InnerVal == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(r.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (r *Redis) UnmarshalJSON(data []byte) error {
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal redis: %v", err)
	}

	var t RedisImpl

	if provider, ok := valueMap["provider"]; ok {
		switch RedisProvider(fmt.Sprintf("%v", provider)) {
		case RedisProviderMiniredis:
			t = &RedisMiniredis{}
		case RedisProviderRedis:
			t = &RedisReal{}
		default:
			return fmt.Errorf("unknown redis provider %v", provider)
		}
	} else {
		// Default to real Redis when no provider specified
		t = &RedisReal{}
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	r.InnerVal = t
	return nil
}
