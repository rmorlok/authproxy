package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "120")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.True(t, ok)
	assert.Equal(t, 120*time.Second, d)
}

func TestParseRetryAfter_ZeroSeconds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "0")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "Thu, 01 Jan 2026 00:02:00 GMT")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.True(t, ok)
	assert.Equal(t, 120*time.Second, d)
}

func TestParseRetryAfter_HTTPDateInPast(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "Thu, 01 Jan 2026 00:02:00 GMT")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRetryAfter_ISO8601(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("X-RateLimit-Reset", "2026-01-01T00:05:00Z")

	d, ok := ParseRetryAfter(headers, []string{"X-RateLimit-Reset"}, now)
	assert.True(t, ok)
	assert.Equal(t, 5*time.Minute, d)
}

func TestParseRetryAfter_MultipleHeaders_FirstWins(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "30")
	headers.Set("X-RateLimit-Reset", "2026-01-01T00:05:00Z")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After", "X-RateLimit-Reset"}, now)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Second, d)
}

func TestParseRetryAfter_FallsThrough(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("X-RateLimit-Reset", "2026-01-01T00:05:00Z")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After", "X-RateLimit-Reset"}, now)
	assert.True(t, ok)
	assert.Equal(t, 5*time.Minute, d)
}

func TestParseRetryAfter_NoHeaders(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}

	_, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.False(t, ok)
}

func TestParseRetryAfter_UnparseableValue(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "not-a-number")

	_, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.False(t, ok)
}

func TestParseRetryAfter_NegativeSeconds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "-5")

	_, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.False(t, ok)
}

func TestParseRetryAfter_Whitespace(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("Retry-After", "  60  ")

	d, ok := ParseRetryAfter(headers, []string{"Retry-After"}, now)
	assert.True(t, ok)
	assert.Equal(t, 60*time.Second, d)
}
