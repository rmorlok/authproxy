package common

import (
	"fmt"

	"code.cloudfoundry.org/bytefmt"
)

type HumanByteSize struct {
	uint64
}

// MarshalJSON provides custom serialization of the duration to a human-readable string (e.g., "2mb").
func (b HumanByteSize) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", bytefmt.ByteSize(b.uint64))), nil
}

// UnmarshalJSON parses a human-readable size string back into bytes of `uint64`.
func (b *HumanByteSize) UnmarshalJSON(data []byte) error {
	// Remove the surrounding quotes from the JSON string
	s := string(data)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("invalid byte size format: %s", s)
	}
	parseBytes, err := bytefmt.ToBytes(s[1 : len(s)-1])
	if err != nil {
		return fmt.Errorf("failed to parse size in bytes: %w", err)
	}
	b.uint64 = parseBytes
	return nil
}

// MarshalYAML provides custom serialization of the bytes to a human-readable string (e.g., "2mb").
func (b HumanByteSize) MarshalYAML() (interface{}, error) {
	return bytefmt.ByteSize(b.uint64), nil // `time.Duration.String()` converts duration to "2m", "4h", etc.
}

// UnmarshalYAML parses a human-readable bytes size string back into `unint64`.
func (b *HumanByteSize) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parseBytes, err := bytefmt.ToBytes(s)
	if err != nil {
		return fmt.Errorf("failed to parse size in bytes: %w", err)
	}
	b.uint64 = parseBytes
	return nil
}

func (b *HumanByteSize) Value() uint64 {
	if b == nil {
		return 0
	}

	return b.uint64
}
