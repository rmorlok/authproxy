package httpcodec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// bodyMode identifies the sole body representation captured by a Body program.
// The zero value is deliberately invalid, distinguishing an uncompiled program
// from a compiled request that intentionally has no body.
type bodyMode string

const (
	bodyAbsent    bodyMode = "absent"
	bodyJSON      bodyMode = "json"
	bodyText      bodyMode = "text"
	bodyBinary    bodyMode = "binary"
	bodyForm      bodyMode = "form"
	bodyMultipart bodyMode = "multipart"
)

// Body is an immutable compiled HTTP body definition. Exactly one of its value,
// text, or form programs is used for a present body. All multipart strings share
// one value program and rendering budget; media types remain literal metadata.
// A Body can be reused concurrently with independent invocation snapshots.
type Body struct {
	mode           bodyMode
	value          *render.ValueTemplate
	text           *render.Template
	form           *Parameters
	contentType    string
	jsonText       bool
	partMediaTypes []string
	limits         render.Limits
}

// RenderedBody contains detached request bytes and the content type selected by
// the codec. Present distinguishes no body from a present zero-byte text, binary,
// or form body. ContentType is empty for absent and raw bodies; raw bodies obtain
// their type from the authored request headers. Request assembly must preserve a
// generated multipart boundary and validate conflicts with authored headers.
type RenderedBody struct {
	Present     bool
	Bytes       []byte
	ContentType string
}

// CompileBody validates and captures only the body fields of an HTTP executor;
// URL, method, headers, query, and response rules belong to request assembly.
// It preserves decoded explicit-null errors and rejects competing body modes.
// Limits must be positive. Source bytes are checked before static validation or
// serialization, then the selected program enforces render depth/work/size caps.
// No invocation data is evaluated and no network or filesystem I/O occurs.
func CompileBody(source *tools.ProxyHTTP, limits render.Limits) (*Body, error) {
	if source == nil {
		return nil, errors.New("HTTP body definition is required")
	}
	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxSteps <= 0 {
		return nil, errors.New("body rendering limits must all be positive")
	}
	if err := checkBodySource(source, limits); err != nil {
		return nil, err
	}
	if err := source.ValidateBody(nil); err != nil {
		return nil, err
	}

	body := &Body{mode: bodyAbsent, limits: limits}
	var err error
	switch {
	case source.BodyJSON != nil:
		body.mode, body.contentType = bodyJSON, "application/json"
		body.value, err = render.CompileValue(source.BodyJSON, limits)
	case source.BodyTemplate != nil:
		body.mode, body.contentType = bodyText, source.BodyTemplate.MediaType
		var mediaType string
		mediaType, err = parseBodyMediaType(body.contentType)
		if err == nil {
			_, subtype, _ := strings.Cut(mediaType, "/")
			body.jsonText = subtype == "json" || strings.HasSuffix(subtype, "+json")
			body.text, err = render.Compile(source.BodyTemplate.Template, limits)
		}
	case source.BodyRaw != nil:
		body.mode = bodyBinary
		body.text, err = render.Compile(*source.BodyRaw, limits)
	case source.Form != nil:
		body.mode, body.contentType = bodyForm, "application/x-www-form-urlencoded"
		body.form, err = CompileParameters(source.Form.Fields, limits)
	case source.Multipart != nil:
		body.mode = bodyMultipart
		err = body.compileMultipart(source.Multipart.Parts)
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

// checkBodySource bounds raw source data before validators decode JSON/base64
// or a compiler allocates its serialized form. Serialized template programs are
// checked again by render, since JSON quoting and field framing add bytes.
func checkBodySource(source *tools.ProxyHTTP, limits render.Limits) error {
	remaining := limits.MaxBytes
	add := func(size int) bool {
		if size > remaining {
			return false
		}
		remaining -= size
		return true
	}
	if !add(len(source.BodyJSON)) {
		return errors.New("body source byte limit exceeded")
	}
	if source.BodyTemplate != nil && (!add(len(source.BodyTemplate.MediaType)) || !add(len(source.BodyTemplate.Template))) {
		return errors.New("body source byte limit exceeded")
	}
	if source.BodyRaw != nil && !add(len(*source.BodyRaw)) {
		return errors.New("body source byte limit exceeded")
	}
	if source.Form != nil {
		if len(source.Form.Fields) > limits.MaxSteps {
			return errors.New("body field count exceeds render step limit")
		}
		for name, value := range source.Form.Fields {
			if !add(len(name)) || !add(len(value)) {
				return errors.New("body source byte limit exceeded")
			}
		}
	}
	if source.Multipart != nil {
		if len(source.Multipart.Parts) > limits.MaxSteps {
			return errors.New("body part count exceeds render step limit")
		}
		for _, part := range source.Multipart.Parts {
			if !add(len(part.Name)) || !add(len(part.Filename)) || !add(len(part.MediaType)) ||
				(part.Text != nil && !add(len(*part.Text))) || (part.BodyRaw != nil && !add(len(*part.BodyRaw))) {
				return errors.New("body source byte limit exceeded")
			}
		}
	}
	return nil
}

// compileMultipart copies the existing part contract and compiles all names,
// filenames, and payloads as one JSON program. Media types are removed from the
// template source and captured separately so quoted MIME parameters never become
// executable Mustache strings. No mutable source pointers survive compilation.
func (b *Body) compileMultipart(source []tools.MultipartPart) error {
	parts := make([]tools.MultipartPart, len(source))
	b.partMediaTypes = make([]string, len(source))
	for i, part := range source {
		// MarshalJSON replaces invalid UTF-8 in Go strings; reject it before
		// serializing so compilation never silently changes authored metadata.
		if !utf8.ValidString(part.Name) || !utf8.ValidString(part.Filename) ||
			(part.Text != nil && !utf8.ValidString(*part.Text)) || (part.BodyRaw != nil && !utf8.ValidString(*part.BodyRaw)) {
			return errors.New("multipart template strings must be valid UTF-8")
		}
		if part.MediaType != "" {
			if _, err := parseBodyMediaType(part.MediaType); err != nil {
				return err
			}
		}
		parts[i] = part
		parts[i].MediaType = ""
		b.partMediaTypes[i] = part.MediaType
	}
	sourceJSON, err := json.Marshal(parts)
	if err != nil {
		return errors.New("cannot compile multipart body")
	}
	b.value, err = render.CompileValue(sourceJSON, b.limits)
	return err
}

// Render prepares a complete body without sending a request. It validates JSON
// text, decodes binary input, and includes multipart framing in the output cap.
// Results own their bytes. Cancellation and failures return a zero RenderedBody
// without partial data or resolved configuration values in encoding errors.
func (b *Body) Render(ctx context.Context, data *render.Context) (RenderedBody, error) {
	if b == nil || b.mode == "" || ctx == nil || data == nil {
		return RenderedBody{}, errors.New("body program, context, and invocation data are required")
	}
	if err := ctx.Err(); err != nil {
		return RenderedBody{}, err
	}
	result := RenderedBody{Present: b.mode != bodyAbsent, ContentType: b.contentType}
	var err error
	switch b.mode {
	case bodyAbsent:
		return result, nil
	case bodyJSON:
		result.Bytes, err = b.value.Render(ctx, data)
	case bodyText, bodyBinary, bodyForm:
		var text string
		if b.mode == bodyForm {
			text, err = b.form.Render(ctx, data)
		} else {
			text, err = b.text.Render(ctx, data)
		}
		if err == nil {
			if b.mode == bodyBinary {
				result.Bytes, err = DecodeBase64(ctx, text, b.limits.MaxBytes, b.limits.MaxBytes)
			} else {
				result.Bytes = []byte(text)
				if b.jsonText && !json.Valid(result.Bytes) {
					err = errors.New("rendered body template must contain valid JSON")
				}
			}
		}
	case bodyMultipart:
		result.Bytes, result.ContentType, err = b.renderMultipart(ctx, data)
	default:
		err = errors.New("invalid compiled body mode")
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return RenderedBody{}, err
	}
	return result, nil
}

// renderMultipart restores literal media types after rendering and delegates
// final MIME validation and encoding to the shared resolved-parts codec.
func (b *Body) renderMultipart(ctx context.Context, data *render.Context) ([]byte, string, error) {
	raw, err := b.value.Render(ctx, data)
	if err != nil {
		return nil, "", err
	}
	var parts []tools.MultipartPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, "", errors.New("rendered multipart body must contain valid parts")
	}
	if len(parts) != len(b.partMediaTypes) {
		return nil, "", errors.New("rendered multipart part count changed")
	}
	for i := range parts {
		parts[i].MediaType = b.partMediaTypes[i]
	}
	return EncodeMultipart(ctx, parts, b.limits.MaxBytes)
}
