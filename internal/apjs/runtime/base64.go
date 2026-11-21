package runtime

import "github.com/dop251/goja"

// base64Alphabet is the standard padded alphabet used by the binary-string
// helpers. URL-safe characters are deliberately not accepted by atob.
const base64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// base64PollMask checks cancellation every 4096 input units or output bytes,
// avoiding context polling on every unit. Each loop also checks its final chunk.
const base64PollMask = 4096 - 1

// atob performs WHATWG forgiving-base64 decoding on one primitive string and
// returns a binary string whose UTF-16 units preserve decoded byte values.
// Input is charged before validation; exact output is reserved before allocation.
// Whitespace and padding are inspected in place so normalization needs no copy.
func (h *utilityHost) atob(call goja.FunctionCall) goja.Value {
	input := h.begin(call)
	length := input.Length()
	dataUnits, padding := 0, 0
	for i := 0; i < length; i++ {
		if i&base64PollMask == 0 {
			h.checkContext()
		}
		unit := input.CharAt(i)
		if base64Whitespace(unit) {
			continue
		}
		if unit == '=' {
			padding++
			if padding > 2 {
				h.invalidInput()
			}
			continue
		}
		if _, valid := base64Digit(unit); !valid || padding != 0 {
			h.invalidInput()
		}
		dataUnits++
	}
	h.checkContext()

	// Padding is optional, but when present it must complete a four-unit
	// group. One remaining data unit cannot supply a complete decoded byte.
	if (padding != 0 && (dataUnits+padding)%4 != 0) || dataUnits%4 == 1 {
		h.invalidInput()
	}
	outputLength := (dataUnits / 4) * 3
	if remainder := dataUnits % 4; remainder != 0 {
		outputLength += remainder - 1
	}
	// StringFromUTF16 may allocate a leading marker in its private copy. Bound
	// both UTF-16 allocations before their byte-size multiplication can overflow.
	if outputLength > int(^uint(0)>>1)/2-1 {
		h.failLimit()
	}
	h.reserveOutput(outputLength)
	output := make([]uint16, outputLength)

	var buffer uint32
	var bits uint
	position := 0
	for i := 0; i < length; i++ {
		if i&base64PollMask == 0 {
			h.checkContext()
		}
		unit := input.CharAt(i)
		if base64Whitespace(unit) {
			continue
		}
		if unit == '=' {
			break
		}
		digit, _ := base64Digit(unit) // The first scan validated every data unit.
		buffer = buffer<<6 | uint32(digit)
		bits += 6
		if bits >= 8 {
			bits -= 8
			output[position] = uint16((buffer >> bits) & 0xff)
			position++
		}
	}
	h.checkContext()
	// Forgiving decoding discards the final two or four bits; unlike strict
	// canonical decoding it accepts both "YQ" and "YR" as the byte for "a".
	result := goja.StringFromUTF16(output)
	h.checkContext()
	return result
}

// btoa encodes one primitive binary string with standard base64 padding. Each
// UTF-16 unit must fit in one byte; no Unicode encoding or authored coercion is
// performed. The complete output allowance is reserved before allocating it.
func (h *utilityHost) btoa(call goja.FunctionCall) goja.Value {
	input := h.begin(call)
	length := input.Length()
	for i := 0; i < length; i++ {
		if i&base64PollMask == 0 {
			h.checkContext()
		}
		if input.CharAt(i) > 0xff {
			h.invalidInput()
		}
	}
	h.checkContext()

	groups := length / 3
	if length%3 != 0 {
		groups++
	}
	if groups > int(^uint(0)>>1)/4 {
		h.failLimit()
	}
	outputLength := groups * 4
	h.reserveOutput(outputLength)
	output := make([]byte, outputLength)

	for source, target := 0, 0; source < length; target += 4 {
		// Output advances by four bytes, so it reaches each polling boundary;
		// the three-unit input stride would skip most of those boundaries.
		if target&base64PollMask == 0 {
			h.checkContext()
		}
		count := min(3, length-source)
		first := input.CharAt(source)
		var second, third uint16
		if count > 1 {
			second = input.CharAt(source + 1)
		}
		if count > 2 {
			third = input.CharAt(source + 2)
		}
		output[target] = base64Alphabet[first>>2]
		output[target+1] = base64Alphabet[(first&3)<<4|second>>4]
		output[target+2], output[target+3] = '=', '='
		if count > 1 {
			output[target+2] = base64Alphabet[(second&15)<<2|third>>6]
		}
		if count > 2 {
			output[target+3] = base64Alphabet[third&63]
		}
		source += count
	}
	h.checkContext()

	result := h.vm.ToValue(string(output))
	h.checkContext()
	return result
}

// base64Whitespace reports precisely the five ASCII whitespace characters
// removed by forgiving-base64 decoding; other Unicode whitespace is invalid.
func base64Whitespace(unit uint16) bool {
	switch unit {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

// base64Digit maps one standard-alphabet UTF-16 unit to its six-bit value.
// Padding, URL-safe characters, and all non-ASCII units report false.
func base64Digit(unit uint16) (byte, bool) {
	switch {
	case unit >= 'A' && unit <= 'Z':
		return byte(unit - 'A'), true
	case unit >= 'a' && unit <= 'z':
		return byte(unit-'a') + 26, true
	case unit >= '0' && unit <= '9':
		return byte(unit-'0') + 52, true
	case unit == '+':
		return 62, true
	case unit == '/':
		return 63, true
	default:
		return 0, false
	}
}
