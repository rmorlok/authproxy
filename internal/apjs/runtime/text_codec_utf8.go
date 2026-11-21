package runtime

import (
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// textCodecPollInterval bounds native work between cancellation checks. Scalar
// readers consume variable-width input, so loops compare distance from the last
// check instead of requiring an index to hit a particular bit-mask boundary.
const textCodecPollInterval = 4096

// encodeUTF8 converts native UTF-16 code units to UTF-8 bytes, replacing unpaired
// surrogates with U+FFFD as TextEncoder's scalar-string conversion requires.
// The caller already charged the utility call and input units. The first pass
// counts exact bytes; output is reserved before allocation and a second pass
// writes into that allocation. The immutable input stays on its VM's goroutine.
func (h *utilityHost) encodeUTF8(input goja.String) []byte {
	h.checkContext()
	length, outputLength := input.Length(), 0
	lastCheck := 0
	for index := 0; index < length; {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed := textUTF16Scalar(input, index, length)
		size := utf8.RuneLen(scalar)
		if outputLength > int(^uint(0)>>1)-size {
			h.failLimit()
		}
		outputLength += size
		index += consumed
	}
	h.checkContext()
	h.reserveOutput(outputLength)
	output := make([]byte, outputLength)

	lastCheck = 0
	for index, target := 0, 0; index < length; {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed := textUTF16Scalar(input, index, length)
		target += utf8.EncodeRune(output[target:], scalar)
		index += consumed
	}
	h.checkContext()
	return output
}

// decodeUTF8 decodes one complete byte sequence with WHATWG UTF-8 replacement
// behavior. Fatal malformed input throws before reserving output. Unless
// ignoreBOM is true, only an initial UTF-8 BOM is removed; every call starts a
// fresh decoding operation. The caller already charged the utility call/input
// bytes and must keep input unchanged while both passes run on the VM owner.
// Output accounting uses exact UTF-16 code units, including surrogate pairs.
func (h *utilityHost) decodeUTF8(input []byte, fatal, ignoreBOM bool) goja.String {
	h.checkContext()
	start := 0
	if !ignoreBOM && len(input) >= 3 && input[0] == 0xef && input[1] == 0xbb && input[2] == 0xbf {
		start = 3
	}

	outputLength, lastCheck := 0, start
	for index := start; index < len(input); {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed, valid := textUTF8Scalar(input, index)
		if !valid && fatal {
			h.invalidTextInput()
		}
		// A supplementary scalar consumes four input bytes and produces two
		// code units; all other results consume at least one byte per unit.
		outputLength++
		if scalar > 0xffff {
			outputLength++
		}
		index += consumed
	}
	h.checkContext()
	// Both this buffer and StringFromUTF16's copy use UTF-16 storage. Goja's
	// copy may also allocate a leading marker, so guard its size multiplication.
	if outputLength > int(^uint(0)>>1)/2-1 {
		h.failLimit()
	}
	h.reserveOutput(outputLength)
	output := make([]uint16, outputLength)

	lastCheck = start
	for index, target := start, 0; index < len(input); {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed, _ := textUTF8Scalar(input, index)
		if scalar > 0xffff {
			leading, trailing := utf16.EncodeRune(scalar)
			output[target], output[target+1] = uint16(leading), uint16(trailing)
			target += 2
		} else {
			output[target] = uint16(scalar)
			target++
		}
		index += consumed
	}
	h.checkContext()
	result := goja.StringFromUTF16(output)
	h.checkContext()
	return result
}

// textUTF16Scalar reads one scalar from a valid index in an immutable native
// string. A surrogate pair consumes two units; each unpaired surrogate consumes
// one and becomes U+FFFD. It never invokes authored coercion or exports a value.
func textUTF16Scalar(input goja.String, index, length int) (rune, int) {
	unit := input.CharAt(index)
	if unit < 0xd800 || unit > 0xdfff {
		return rune(unit), 1
	}
	if unit <= 0xdbff && index+1 < length {
		next := input.CharAt(index + 1)
		if next >= 0xdc00 && next <= 0xdfff {
			return utf16.DecodeRune(rune(unit), rune(next)), 2
		}
	}
	return utf8.RuneError, 1
}

// textUTF8Scalar reads one scalar or malformed maximal subpart at a valid byte
// index. A malformed prefix becomes U+FFFD and reports false; the first byte
// that violates its continuation bounds is not consumed and is processed again
// by the next call. Truncated valid prefixes consume their remaining bytes as
// one error. Unlike a DecodeRune loop, this matches WHATWG replacement grouping.
func textUTF8Scalar(input []byte, index int) (scalar rune, consumed int, valid bool) {
	first := input[index]
	if first < 0x80 {
		return rune(first), 1, true
	}

	width := 0
	lower, upper := byte(0x80), byte(0xbf)
	switch {
	case first >= 0xc2 && first <= 0xdf:
		width, scalar = 2, rune(first&0x1f)
	case first >= 0xe0 && first <= 0xef:
		width, scalar = 3, rune(first&0x0f)
		if first == 0xe0 {
			lower = 0xa0 // Reject overlong encodings.
		} else if first == 0xed {
			upper = 0x9f // Reject UTF-16 surrogate code points.
		}
	case first >= 0xf0 && first <= 0xf4:
		width, scalar = 4, rune(first&0x07)
		if first == 0xf0 {
			lower = 0x90 // Reject overlong encodings.
		} else if first == 0xf4 {
			upper = 0x8f // Stop at Unicode's maximum scalar, U+10FFFF.
		}
	default:
		return utf8.RuneError, 1, false
	}

	for consumed = 1; consumed < width; consumed++ {
		if consumed >= len(input)-index {
			return utf8.RuneError, consumed, false
		}
		next := input[index+consumed]
		if next < lower || next > upper {
			return utf8.RuneError, consumed, false
		}
		scalar = scalar<<6 | rune(next&0x3f)
		lower, upper = 0x80, 0xbf
	}
	return scalar, consumed, true
}
