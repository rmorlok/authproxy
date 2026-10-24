package httpcodec

import (
	"context"
	"encoding/base64"
	"errors"
)

// DecodeBase64 decodes standard padded base64 without interpreting its contents
// as a template or filename. Both byte limits must be positive. Encoded input,
// including ignored CR/LF characters, is checked before allocation; the decoded
// limit counts actual bytes rather than the encoding's padded upper estimate.
// Strict decoding rejects nonzero padding bits. Errors return no partial data
// and never include the supplied data.
func DecodeBase64(ctx context.Context, encoded string, maxEncodedBytes, maxDecodedBytes int) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("base64 decoding context is required")
	}
	if maxEncodedBytes <= 0 || maxDecodedBytes <= 0 {
		return nil, errors.New("base64 byte limits must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(encoded) > maxEncodedBytes {
		return nil, errors.New("base64 encoded byte limit exceeded")
	}

	// Standard base64 ignores CR and LF even in strict mode. Count its real
	// symbols and trailing padding without first allocating a stripped copy.
	count := 0
	var previous, last byte
	for i := 0; i < len(encoded); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if encoded[i] != '\r' && encoded[i] != '\n' {
			count++
			previous, last = last, encoded[i]
		}
	}
	if count%4 != 0 {
		return nil, errors.New("body must contain valid standard base64")
	}
	decodedLength := count / 4 * 3
	if count > 0 && last == '=' {
		decodedLength--
		if previous == '=' {
			decodedLength--
		}
	}
	if decodedLength > maxDecodedBytes {
		return nil, errors.New("base64 decoded byte limit exceeded")
	}

	decoded := make([]byte, decodedLength)
	n, err := base64.StdEncoding.Strict().Decode(decoded, []byte(encoded))
	if err != nil {
		return nil, errors.New("body must contain valid standard base64")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return decoded[:n], nil
}
