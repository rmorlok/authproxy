package util

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseHTTPStatusCodeRange parses a single status code, an inclusive range, or
// a status family shorthand such as 2xx.
func ParseHTTPStatusCodeRange(r string) (int, int, error) {
	normalized := strings.ToLower(strings.TrimSpace(r))
	if len(normalized) == 3 && normalized[1:] == "xx" {
		family, err := strconv.Atoi(normalized[:1])
		if err != nil || family < 1 || family > 5 {
			return 0, 0, fmt.Errorf("invalid HTTP status code range %q", r)
		}
		return family * 100, family*100 + 99, nil
	}

	start, end, err := ParseIntegerRange(normalized, 100, 599)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}
