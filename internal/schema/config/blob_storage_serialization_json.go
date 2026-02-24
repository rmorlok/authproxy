package config

import (
	"encoding/json"
	"fmt"
)

func (b *BlobStorage) MarshalJSON() ([]byte, error) {
	if b == nil || b.InnerVal == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(b.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (b *BlobStorage) UnmarshalJSON(data []byte) error {
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal blob storage: %v", err)
	}

	var t BlobStorageImpl

	if provider, ok := valueMap["provider"]; ok {
		switch BlobStorageProvider(fmt.Sprintf("%v", provider)) {
		case BlobStorageProviderMemory:
			t = &BlobStorageMemory{}
		case BlobStorageProviderS3:
			t = &BlobStorageS3{}
		default:
			return fmt.Errorf("unknown blob storage provider %v", provider)
		}
	} else {
		// Default to S3 when no provider specified
		t = &BlobStorageS3{}
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	b.InnerVal = t
	return nil
}
