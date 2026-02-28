package util

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseIntegerRange parses a string representing a single integer or a range of integers
// separated by a dash and returns the start and end values. If a single integer is provided,
// start and end will be equal. Both values are validated against the provided min/max bounds.
//
// Examples:
//
//	ParseIntegerRange("200", 100, 599)       => 200, 200, nil
//	ParseIntegerRange("200-299", 100, 599)   => 200, 299, nil
//	ParseIntegerRange("", 100, 599)           => error (empty input)
//	ParseIntegerRange("1-2-3", 100, 599)      => error (more than one dash)
//	ParseIntegerRange("50", 100, 599)          => error (out of bounds)
func ParseIntegerRange(r string, minValidValue, maxValidValue int) (int, int, error) {
	if r == "" {
		return 0, 0, errors.New("no value specified for integer code range")
	}

	parts := strings.Split(r, "-")

	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid integer code range format: '%s'; more than one dash", r)
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid integer code range format: '%s'; cannot parse value as integer", r)
	}

	end := 0
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid integer code range format: '%s'; cannot parse value as integer", r)
		}
	} else {
		end = start
	}

	if start < minValidValue || start > maxValidValue {
		return 0, 0, fmt.Errorf("invalid integer code range format: '%s'; start value must be between %d and %d", r, minValidValue, maxValidValue)
	}

	if end < minValidValue || end > maxValidValue {
		return 0, 0, fmt.Errorf("invalid integer code range format: '%s'; end value must be between %d and %d", r, minValidValue, maxValidValue)
	}

	return start, end, nil
}
