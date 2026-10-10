package toolsets

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeJSONInteger accepts exact integer values independently of their
// JSON spelling and leaves the caller's original bytes unchanged.
func TestNormalizeJSONInteger(t *testing.T) {
	for _, test := range []struct {
		input, want string
	}{
		{"0", "0"},
		{"-0", "0"},
		{"1.0", "1"},
		{"1e0", "1"},
		{"1E+0", "1"},
		{"10e-1", "1"},
		{"-15e-1", ""},
		{"-1.5e1", "-15"},
		{"1.2300e2", "123"},
		{"12300.0e-2", "123"},
		{"0.00100e3", "1"},
		{"100e1", "1000"},
		{"1e00000000000000000000000000000000000000000000000000000000", "1"},
		{" \n 42.000e+0\t", "42"},
		{"0.5", ""},
		{"1.01", ""},
		{"1e-1", ""},
		{"100e-3", ""},
		{`"1"`, ""},
		{`"1.0"`, ""},
		{"null", ""},
		{"true", ""},
		{"false", ""},
		{"[]", ""},
		{"{}", ""},
	} {
		t.Run(test.input, func(t *testing.T) {
			raw := json.RawMessage(test.input)
			require.True(t, json.Valid(raw), "fixture follows the helper's JSON precondition")
			got, err := normalizeJSONInteger(raw)
			require.Equal(t, test.input, string(raw))
			if test.want == "" {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, string(got))
		})
	}
}

// TestNormalizeJSONIntegerPlatformBounds checks signed int limits without
// float rounding, including fractional values that are extremely close to them.
func TestNormalizeJSONIntegerPlatformBounds(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	maximum := strconv.Itoa(maxInt)
	minimum := strconv.Itoa(-maxInt - 1)
	positiveOverflow := strconv.FormatUint(uint64(maxInt)+1, 10)
	negativeOverflow := "-" + strconv.FormatUint(uint64(maxInt)+2, 10)
	for _, test := range []struct {
		name, input, want string
	}{
		{"maximum", maximum, maximum},
		{"minimum", minimum, minimum},
		{"maximum decimal", maximum + ".000", maximum},
		{"minimum decimal", minimum + ".0", minimum},
		{"maximum cancellation", maximum + "00e-2", maximum},
		{"minimum cancellation", minimum + "00e-2", minimum},
		{"positive overflow", positiveOverflow, ""},
		{"negative overflow", negativeOverflow, ""},
		{"overflow decimal", positiveOverflow + ".0", ""},
		{"overflow cancellation", positiveOverflow + "0e-1", ""},
		{"maximum near fraction", maximum + ".000000000000000000001", ""},
		{"minimum near fraction", minimum + ".000000000000000000001", ""},
		{"boundary exponent fraction", maximum + "1e-1", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeJSONInteger(json.RawMessage(test.input))
			if test.want == "" {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, string(got))
		})
	}
	if strconv.IntSize == 64 {
		// This integer is not exactly representable by a float64.
		got, err := normalizeJSONInteger(json.RawMessage("9007199254740993.0"))
		require.NoError(t, err)
		require.Equal(t, "9007199254740993", string(got))
	}
}

// TestNormalizeJSONIntegerExtremeExponents exercises bounded exponent handling
// and long coefficients whose zeros cancel without constructing huge numbers.
func TestNormalizeJSONIntegerExtremeExponents(t *testing.T) {
	hugeExponent := strings.Repeat("9", 4096)
	zeros := strings.Repeat("0", 4096)
	for _, test := range []struct {
		name, input, want string
	}{
		{"positive magnitude", "1e" + hugeExponent, ""},
		{"negative magnitude", "1e-" + hugeExponent, ""},
		{"negative coefficient", "-1e" + hugeExponent, ""},
		{"zero positive exponent", "0e+" + hugeExponent, "0"},
		{"zero negative exponent", "-0.000e-" + hugeExponent, "0"},
		{"positive int exponent boundary", "1e" + strconv.Itoa(int(^uint(0)>>1)), ""},
		{"negative int exponent boundary", "1e" + strconv.Itoa(-int(^uint(0)>>1)-1), ""},
		{"coefficient cancellation", "1" + zeros + "e-4096", "1"},
		{"decimal cancellation", "0." + zeros + "1e4097", "1"},
		{"one decimal place remains", "0." + zeros + "1e4096", ""},
		{"exponent leading zeros", "1e+" + zeros + "1", "10"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(test.input)
			require.True(t, json.Valid(raw))
			got, err := normalizeJSONInteger(raw)
			if test.want == "" {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, string(got))
		})
	}
}
