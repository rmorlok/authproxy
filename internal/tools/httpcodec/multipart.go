package httpcodec

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/textproto"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// EncodeMultipart encodes already-rendered fields and binary parts in input
// order, preserving repeated names. It does not interpret templates or read
// filenames. Each part must have exactly one text or base64 body. Text defaults
// to text/plain and binary to application/octet-stream unless a concrete media
// type is supplied. Names and filenames are quoted UTF-8 metadata.
//
// maxBytes must be positive and separately caps aggregate source string bytes
// and the complete encoded body, including headers and boundaries. The returned
// content type contains the generated boundary and must accompany those bytes.
// Cancellation or any error returns neither partial bytes nor a content type.
func EncodeMultipart(ctx context.Context, parts []tools.MultipartPart, maxBytes int) ([]byte, string, error) {
	if err := validateMultipartSource(ctx, parts, maxBytes); err != nil {
		return nil, "", err
	}
	output := multipartBuffer{ctx: ctx, maxBytes: maxBytes}
	writer := multipart.NewWriter(&output)
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		contentType := part.MediaType
		var payload []byte
		if part.Text != nil {
			payload = []byte(*part.Text)
			if contentType == "" {
				contentType = "text/plain"
			}
		} else {
			var err error
			payload, err = DecodeBase64(ctx, *part.BodyRaw, maxBytes, maxBytes)
			if err != nil {
				return nil, "", err
			}
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}

		// Quote parameters directly, as multipart.Writer does, so Unicode
		// filenames stay ordinary filename parameters rather than filename*.
		disposition := `form-data; name="` + quoteMultipartParameter(part.Name) + `"`
		if part.Filename != "" {
			disposition += `; filename="` + quoteMultipartParameter(part.Filename) + `"`
		}
		header := textproto.MIMEHeader{
			"Content-Disposition": {disposition},
			"Content-Type":        {contentType},
		}
		partWriter, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := partWriter.Write(payload); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return output.Bytes(), writer.FormDataContentType(), nil
}

// validateMultipartSource bounds all metadata and payload input before any
// decoding or MIME allocation, then checks every part's inert source contract.
func validateMultipartSource(ctx context.Context, parts []tools.MultipartPart, maxBytes int) error {
	if ctx == nil {
		return errors.New("multipart encoding context is required")
	}
	if maxBytes <= 0 {
		return errors.New("multipart byte limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(parts) == 0 {
		return errors.New("multipart body must contain at least one part")
	}
	// Every valid part requires a nonempty name, so count alone can exceed
	// the source cap without traversing or allocating anything per part.
	if len(parts) > maxBytes {
		return errors.New("multipart source byte limit exceeded")
	}
	remaining := maxBytes
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		lengths := [5]int{len(part.Name), len(part.Filename), len(part.MediaType)}
		if part.Text != nil {
			lengths[3] = len(*part.Text)
		}
		if part.BodyRaw != nil {
			lengths[4] = len(*part.BodyRaw)
		}
		for _, length := range lengths {
			if length > remaining {
				return errors.New("multipart source byte limit exceeded")
			}
			remaining -= length
		}
	}
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.TrimSpace(part.Name) == "" || !validPartMetadata(part.Name) || !validPartMetadata(part.Filename) {
			return errors.New("multipart names and filenames must be valid UTF-8 without control characters; names must be nonempty")
		}
		if part.MediaType != "" {
			if _, err := parseBodyMediaType(part.MediaType); err != nil {
				return err
			}
		}
		if (part.Text == nil) == (part.BodyRaw == nil) {
			return errors.New("multipart parts must contain exactly one text or base64 body")
		}
		if part.Text != nil && !utf8.ValidString(*part.Text) {
			return errors.New("multipart text must be valid UTF-8")
		}
	}
	return nil
}

// quoteMultipartParameter escapes the two characters with syntax inside a
// quoted Content-Disposition value. Metadata controls are rejected beforehand.
func quoteMultipartParameter(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

// multipartBuffer caps every write, including MIME framing added by the
// standard writer. Its bytes are exposed only after the entire body succeeds.
type multipartBuffer struct {
	bytes.Buffer
	ctx      context.Context
	maxBytes int
}

// Write checks cancellation and remaining space before retaining any bytes.
func (b *multipartBuffer) Write(value []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if len(value) > b.maxBytes-b.Len() {
		return 0, errors.New("encoded multipart byte limit exceeded")
	}
	return b.Buffer.Write(value)
}
