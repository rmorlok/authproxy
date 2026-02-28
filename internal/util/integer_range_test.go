package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIntegerRange_SingleValue(t *testing.T) {
	t.Parallel()

	start, end, err := ParseIntegerRange("200", 100, 599)
	require.NoError(t, err)
	require.Equal(t, 200, start)
	require.Equal(t, 200, end)
}

func TestParseIntegerRange_Range(t *testing.T) {
	t.Parallel()

	start, end, err := ParseIntegerRange("200-299", 100, 599)
	require.NoError(t, err)
	require.Equal(t, 200, start)
	require.Equal(t, 299, end)
}

func TestParseIntegerRange_BoundaryValues(t *testing.T) {
	t.Parallel()

	start, end, err := ParseIntegerRange("100-599", 100, 599)
	require.NoError(t, err)
	require.Equal(t, 100, start)
	require.Equal(t, 599, end)
}

func TestParseIntegerRange_Empty(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no value specified")
}

func TestParseIntegerRange_MultipleDashes(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("1-2-3", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than one dash")
}

func TestParseIntegerRange_NonNumericStart(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("abc-200", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse value as integer")
}

func TestParseIntegerRange_NonNumericEnd(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("200-abc", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse value as integer")
}

func TestParseIntegerRange_StartBelowMin(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("50", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start value must be between")
}

func TestParseIntegerRange_StartAboveMax(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("600", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start value must be between")
}

func TestParseIntegerRange_EndBelowMin(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("200-50", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "end value must be between")
}

func TestParseIntegerRange_EndAboveMax(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("200-600", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "end value must be between")
}

func TestParseIntegerRange_SingleValueNonNumeric(t *testing.T) {
	t.Parallel()

	_, _, err := ParseIntegerRange("abc", 100, 599)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse value as integer")
}
