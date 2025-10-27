package common

import (
	"fmt"
	"regexp"
	"strings"

	gohumanize "github.com/dustin/go-humanize"
)

const (
	_          = iota
	KiB uint64 = 1 << (10 * iota)
	MiB
	GiB
	TiB
	PiB
	EiB
)

var decimalRegex = regexp.MustCompile(`\.0+[^0-9]`)

func useEIC(size uint64) bool {
	if size > EiB {
		return size%EiB == 0
	} else if size > PiB {
		return size%PiB == 0
	} else if size > TiB {
		return size%TiB == 0
	} else if size > GiB {
		return size%GiB == 0
	} else if size > MiB {
		return size%MiB == 0
	} else if size > KiB {
		return size%KiB == 0
	}

	return false
}

// HumanByteSize is a wrapper around uint64 that provides custom serialization to a human-readable string (e.g., "2mb").
// For kb, mb, etc it will use base 10 parsing (1kb = 1000 bytes). For bases 2 parsing using IEC (2kib = 1024 bytes).
type HumanByteSize struct {
	uint64
}

// MarshalJSON provides custom serialization of the duration to a human-readable string (e.g., "2mb").
func (b HumanByteSize) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", b.String())), nil
}

func (b HumanByteSize) String() string {
	strVal := gohumanize.Bytes(b.uint64)
	if useEIC(b.uint64) {
		strVal = gohumanize.IBytes(b.uint64)
	}

	// Go humanize puts .0 on whole numbers
	strVal = decimalRegex.ReplaceAllString(strVal, "")

	// Go humanize puts a space and uses capital letters for the units
	strVal = strings.ToLower(strings.Replace(strVal, " ", "", -1))

	return strVal
}

// UnmarshalJSON parses a human-readable size string back into bytes of `uint64`.
func (b *HumanByteSize) UnmarshalJSON(data []byte) error {
	// Remove the surrounding quotes from the JSON string
	s := string(data)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("invalid byte size format: %s", s)
	}
	parseBytes, err := gohumanize.ParseBytes(s[1 : len(s)-1])
	if err != nil {
		return fmt.Errorf("failed to parse size in bytes: %w", err)
	}
	b.uint64 = parseBytes
	return nil
}

// MarshalYAML provides custom serialization of the bytes to a human-readable string (e.g., "2mb").
func (b HumanByteSize) MarshalYAML() (interface{}, error) {
	return b.String(), nil // `time.Duration.String()` converts duration to "2m", "4h", etc.
}

// UnmarshalYAML parses a human-readable bytes size string back into `unint64`.
func (b *HumanByteSize) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parseBytes, err := gohumanize.ParseBytes(s)
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
