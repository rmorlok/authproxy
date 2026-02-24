package config

import (
	"encoding/json"
	"fmt"
)

func (c *AwsCredentials) MarshalJSON() ([]byte, error) {
	if c == nil || c.InnerVal == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(c.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (c *AwsCredentials) UnmarshalJSON(data []byte) error {
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal blob storage credentials: %v", err)
	}

	var t AwsCredentialsImpl

	if credType, ok := valueMap["type"]; ok {
		switch AwsCredentialsType(fmt.Sprintf("%v", credType)) {
		case AwsCredentialsTypeAccessKey:
			t = &AwsCredentialsAccessKey{}
		case AwsCredentialsTypeImplicit:
			t = &AwsCredentialsImplicit{}
		default:
			return fmt.Errorf("unknown blob storage credentials type %v", credType)
		}
	} else {
		// Default to implicit when no type specified
		t = &AwsCredentialsImplicit{}
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	c.InnerVal = t
	return nil
}
