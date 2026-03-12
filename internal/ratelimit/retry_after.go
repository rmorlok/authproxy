package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter checks the given response headers for retry-after information.
// It iterates through headerNames in order and returns the first successfully parsed duration.
// Supported formats:
//   - Integer seconds (e.g., "120")
//   - HTTP-date per RFC 7231 (e.g., "Fri, 31 Dec 1999 23:59:59 GMT")
//   - ISO 8601 timestamp (e.g., "2026-01-01T01:01:01Z") used by Atlassian X-RateLimit-Reset
//
// Returns the parsed duration and true if a valid value was found, or (0, false) if not.
func ParseRetryAfter(headers http.Header, headerNames []string, now time.Time) (time.Duration, bool) {
	for _, name := range headerNames {
		value := headers.Get(name)
		if value == "" {
			continue
		}

		value = strings.TrimSpace(value)

		if d, ok := parseAsSeconds(value); ok {
			return d, true
		}

		if d, ok := parseAsHTTPDate(value, now); ok {
			return d, true
		}

		if d, ok := parseAsISO8601(value, now); ok {
			return d, true
		}
	}

	return 0, false
}

func parseAsSeconds(value string) (time.Duration, bool) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	if seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func parseAsHTTPDate(value string, now time.Time) (time.Duration, bool) {
	t, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	d := t.Sub(now)
	if d < 0 {
		d = 0
	}
	return d, true
}

var iso8601Formats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05",
}

func parseAsISO8601(value string, now time.Time) (time.Duration, bool) {
	for _, format := range iso8601Formats {
		t, err := time.Parse(format, value)
		if err == nil {
			d := t.Sub(now)
			if d < 0 {
				d = 0
			}
			return d, true
		}
	}
	return 0, false
}
