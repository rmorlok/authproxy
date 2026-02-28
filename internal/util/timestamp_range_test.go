package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseTimestampRange_Valid(t *testing.T) {
	t.Parallel()

	start, end, err := ParseTimestampRange("2024-01-15T10:30:00Z-2024-02-20T14:45:00Z")
	require.NoError(t, err)
	require.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), start)
	require.Equal(t, time.Date(2024, 2, 20, 14, 45, 0, 0, time.UTC), end)
}

func TestParseTimestampRange_SameDay(t *testing.T) {
	t.Parallel()

	start, end, err := ParseTimestampRange("2024-06-01T00:00:00Z-2024-06-01T23:59:59Z")
	require.NoError(t, err)
	require.Equal(t, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), start)
	require.Equal(t, time.Date(2024, 6, 1, 23, 59, 59, 0, time.UTC), end)
}

func TestParseTimestampRange_Empty(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTimestampRange("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no value specified")
}

func TestParseTimestampRange_NoDash(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTimestampRange("notarange")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no range separator")
}

func TestParseTimestampRange_WrongDashCount(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTimestampRange("2024-01-15T10:30:00Z")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid timestamp range format")
}

func TestParseTimestampRange_InvalidStartTimestamp(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTimestampRange("2024-99-15T10:30:00Z-2024-02-20T14:45:00Z")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid start timestamp format")
}

func TestParseTimestampRange_InvalidEndTimestamp(t *testing.T) {
	t.Parallel()

	_, _, err := ParseTimestampRange("2024-01-15T10:30:00Z-2024-99-20T14:45:00Z")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid end timestamp format")
}

func TestParseTimestampRange_WithTimezoneOffset(t *testing.T) {
	t.Parallel()

	start, end, err := ParseTimestampRange("2024-03-10T08:00:00+05:00-2024-03-10T12:00:00+05:00")
	require.NoError(t, err)

	loc := time.FixedZone("", 5*60*60)
	require.Equal(t, time.Date(2024, 3, 10, 8, 0, 0, 0, loc), start)
	require.Equal(t, time.Date(2024, 3, 10, 12, 0, 0, 0, loc), end)
}
