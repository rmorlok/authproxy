package config

import (
	"encoding/json"
	"fmt"
)

// extractJsonKey parses a JSON string and extracts a specific key's value as bytes.
func extractJsonKey(jsonStr string, key string) ([]byte, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse secret as JSON: %w", err)
	}

	val, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret JSON", key)
	}

	strVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("key %q in secret JSON is not a string", key)
	}

	return []byte(strVal), nil
}
