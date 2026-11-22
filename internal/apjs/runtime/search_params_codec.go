package runtime

import (
	"unicode/utf8"

	"github.com/dop251/goja"
)

// searchParam is one ordered query tuple. Both strings contain Unicode scalar
// values, with lone UTF-16 surrogates replaced before the tuple is retained.
// Repeated names and empty names/values are preserved independently.
type searchParam struct {
	name  goja.String // Immutable normalized query name owned by this VM.
	value goja.String // Immutable normalized query value owned by this VM.
}

// searchParamString normalizes an admitted primitive string to scalar values.
// The intermediate UTF-8 bytes and retained UTF-16 string both consume output
// allowance before allocation. This keeps normalization independent of whether
// the caller selected the public TextEncoder/TextDecoder bindings.
func (h *utilityHost) searchParamString(input goja.String) goja.String {
	return h.decodeUTF8(h.encodeUTF8(input), false, true)
}

// parseSearchParams parses an admitted query string using the URL Standard's
// application/x-www-form-urlencoded rules. Only an initial question mark is
// removed; this is not a full URL parser. Empty ampersand-delimited segments are
// ignored, and the first equals sign separates each name from its value.
func (h *utilityHost) parseSearchParams(input goja.String) []searchParam {
	// This owned byte buffer is the only scratch space used by percent decoding.
	// Its entire size is reserved before allocation, including an initial '?'.
	encoded := h.encodeUTF8(input)
	if len(encoded) > 0 && encoded[0] == '?' {
		encoded = encoded[1:]
	}

	// Count real tuples before allocating metadata. One output unit per tuple
	// bounds empty names/values even though those strings consume no units.
	count, start := 0, 0
	for index := 0; index < len(encoded); index++ {
		if index&base64PollMask == 0 {
			h.checkContext()
		}
		if encoded[index] == '&' {
			if index > start {
				count++
			}
			start = index + 1
		}
	}
	if start < len(encoded) {
		count++
	}
	h.checkContext()
	h.reserveOutput(count)
	// A tuple holds two interfaces. Guard the allocation's byte multiplication
	// independently of trusted policy values near the platform integer limit.
	if count > int(^uint(0)>>1)/32 {
		h.failLimit()
	}
	pairs := make([]searchParam, 0, count)

	start, equals := 0, -1
	for index := 0; index <= len(encoded); index++ {
		if index&base64PollMask == 0 {
			h.checkContext()
		}
		if index < len(encoded) && encoded[index] != '&' {
			if encoded[index] == '=' && equals < 0 {
				equals = index
			}
			continue
		}
		if index > start {
			nameEnd, valueStart := equals, equals+1
			if equals < 0 {
				nameEnd, valueStart = index, index
			}
			pairs = append(pairs, searchParam{
				name:  h.decodeSearchParam(encoded[start:nameEnd]),
				value: h.decodeSearchParam(encoded[valueStart:index]),
			})
		}
		start, equals = index+1, -1
	}
	h.checkContext()
	return pairs
}

// decodeSearchParam rewrites one owned field in place, converting plus to space
// and valid percent escapes to bytes. Invalid escapes remain literal. Decoding
// uses UTF-8 replacement semantics and preserves a leading BOM as query data.
func (h *utilityHost) decodeSearchParam(input []byte) goja.String {
	target, lastCheck := 0, 0
	for source := 0; source < len(input); source++ {
		if source-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = source
		}
		value := input[source]
		switch {
		case value == '+':
			value = ' '
		case value == '%' && len(input)-source >= 3:
			first, firstOK := searchParamHex(input[source+1])
			second, secondOK := searchParamHex(input[source+2])
			if firstOK && secondOK {
				value = first<<4 | second
				source += 2
			}
		}
		input[target] = value
		target++
	}
	h.checkContext()
	return h.decodeUTF8(input[:target], false, true)
}

// serializeSearchParams emits an ASCII form-query string without an initial
// question mark. The caller has admitted scanning the retained tuple state.
// The first pass reserves the exact result size, and the second fills one owned
// buffer without allocating intermediate encodings of individual fields.
func (h *utilityHost) serializeSearchParams(pairs []searchParam) goja.Value {
	length := 0
	for index, pair := range pairs {
		if index&base64PollMask == 0 {
			h.checkContext()
		}
		separatorLength := 1 // Every tuple includes '=' even when its value is empty.
		if index > 0 {
			separatorLength++
		}
		length = h.searchParamAddLength(length, separatorLength)
		length = h.searchParamAddLength(length, h.searchParamEncodedLength(pair.name))
		length = h.searchParamAddLength(length, h.searchParamEncodedLength(pair.value))
	}
	h.reserveOutput(length)
	output := make([]byte, length)
	position := 0
	for index, pair := range pairs {
		if index&base64PollMask == 0 {
			h.checkContext()
		}
		if index > 0 {
			output[position] = '&'
			position++
		}
		position = h.writeSearchParam(output, position, pair.name)
		output[position] = '='
		position++
		position = h.writeSearchParam(output, position, pair.value)
	}
	h.checkContext()
	result := h.vm.ToValue(string(output))
	h.checkContext()
	return result
}

// searchParamEncodedLength counts one field's serialized ASCII bytes without
// constructing its UTF-8 representation. Scalar iteration replaces any lone
// surrogates defensively, even though stored tuples are already normalized.
func (h *utilityHost) searchParamEncodedLength(input goja.String) int {
	length, lastCheck := 0, 0
	for index := 0; index < input.Length(); {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed := textUTF16Scalar(input, index, input.Length())
		size := 1
		if scalar != ' ' && !searchParamUnescaped(scalar) {
			size = 3 * utf8.RuneLen(scalar)
		}
		length = h.searchParamAddLength(length, size)
		index += consumed
	}
	h.checkContext()
	return length
}

// writeSearchParam fills reserved output with one encoded field and returns the
// next write offset. Spaces become '+', while every escaped UTF-8 byte receives
// an uppercase percent escape. The scratch scalar buffer has constant size.
func (h *utilityHost) writeSearchParam(output []byte, position int, input goja.String) int {
	const hex = "0123456789ABCDEF"
	var scalarBytes [utf8.UTFMax]byte
	lastCheck := 0
	for index := 0; index < input.Length(); {
		if index-lastCheck >= textCodecPollInterval {
			h.checkContext()
			lastCheck = index
		}
		scalar, consumed := textUTF16Scalar(input, index, input.Length())
		switch {
		case scalar == ' ':
			output[position] = '+'
			position++
		case searchParamUnescaped(scalar):
			output[position] = byte(scalar)
			position++
		default:
			count := utf8.EncodeRune(scalarBytes[:], scalar)
			for _, value := range scalarBytes[:count] {
				output[position], output[position+1], output[position+2] = '%', hex[value>>4], hex[value&15]
				position += 3
			}
		}
		index += consumed
	}
	h.checkContext()
	return position
}

// searchParamAddLength prevents integer overflow while counting reserved bytes.
func (h *utilityHost) searchParamAddLength(length, additional int) int {
	if additional > int(^uint(0)>>1)-length {
		h.failLimit()
	}
	return length + additional
}

// searchParamUnescaped identifies the form serializer's exact ASCII allowlist.
// Unlike encodeURIComponent, it escapes '~', '!', apostrophe, and parentheses.
func searchParamUnescaped(scalar rune) bool {
	return scalar >= 'a' && scalar <= 'z' || scalar >= 'A' && scalar <= 'Z' ||
		scalar >= '0' && scalar <= '9' || scalar == '*' || scalar == '-' || scalar == '.' || scalar == '_'
}

// searchParamHex decodes one ASCII hexadecimal digit for forgiving percent input.
func searchParamHex(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
