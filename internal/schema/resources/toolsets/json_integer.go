package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// normalizeJSONInteger returns the canonical decimal spelling of an exact
// integer-valued JSON number that fits a platform int. The caller must supply
// syntactically valid JSON; other JSON types are rejected. Negative values are
// preserved so field-specific validation can apply its own minimum.
//
// Work and temporary storage are bounded by the input length. Exponents only
// affect bounds checks: the function never allocates their implied magnitude.
func normalizeJSONInteger(raw json.RawMessage) (json.RawMessage, error) {
	number := string(bytes.TrimSpace(raw))
	if number == "" || (number[0] != '-' && (number[0] < '0' || number[0] > '9')) {
		return nil, fmt.Errorf("must be a JSON number")
	}

	negative := number[0] == '-'
	if negative {
		number = number[1:]
	}
	mantissa, exponentText := number, "0"
	if at := strings.IndexAny(number, "eE"); at >= 0 {
		mantissa, exponentText = number[:at], number[at+1:]
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		// Zero is representable even when its exponent is too large to parse.
		return json.RawMessage("0"), nil
	}

	// Removing coefficient zeros makes the remaining decimal shift decisive:
	// a negative shift is fractional; a nonnegative shift appends only zeros.
	significant := strings.TrimRight(digits, "0")
	trailingZeros := len(digits) - len(significant)
	minimumExponent := len(fraction) - trailingZeros
	maxInt := int(^uint(0) >> 1)
	maxDigits := len(strconv.Itoa(maxInt))
	if len(significant) > maxDigits {
		return nil, fmt.Errorf("must be an integer within the %d-bit int range", strconv.IntSize)
	}

	exponent, err := strconv.Atoi(exponentText)
	if err != nil {
		// A nonzero number cannot offset an exponent beyond int range with
		// coefficient zeros or decimal places, whose counts fit in its input.
		return nil, fmt.Errorf("must be an integer within the %d-bit int range", strconv.IntSize)
	}
	if exponent < minimumExponent {
		return nil, fmt.Errorf("must be an integer")
	}
	maxShift := maxDigits - len(significant)
	// Check the small permitted shift before subtraction or expansion. The
	// guard also keeps minimumExponent+maxShift safe at the int boundary.
	if minimumExponent <= maxInt-maxShift && exponent > minimumExponent+maxShift {
		return nil, fmt.Errorf("must be an integer within the %d-bit int range", strconv.IntSize)
	}
	shift := exponent - minimumExponent
	normalized := significant + strings.Repeat("0", shift)
	if negative {
		normalized = "-" + normalized
	}
	// Decimal width alone cannot distinguish MaxInt from its next value or
	// account for the extra representable negative magnitude, so check exactly.
	if _, err := strconv.ParseInt(normalized, 10, strconv.IntSize); err != nil {
		return nil, fmt.Errorf("must be an integer within the %d-bit int range", strconv.IntSize)
	}
	return json.RawMessage(normalized), nil
}
