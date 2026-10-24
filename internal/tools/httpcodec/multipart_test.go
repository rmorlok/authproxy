package httpcodec

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// multipartText returns a string pointer for constructing explicit empty and
// nonempty multipart payloads in codec tests.
func multipartText(value string) *string { return &value }

// TestEncodeMultipartRoundTrip checks ordered repeated fields, binary files,
// Unicode metadata, quoting, and default versus explicit part media types.
func TestEncodeMultipartRoundTrip(t *testing.T) {
	// Already-resolved template-like strings stay inert, including file paths.
	parts := []tools.MultipartPart{
		{Name: "tag", Text: multipartText("Ünïcödé note\nline")},
		{Name: "tag", Text: multipartText("")},
		{Name: `a"\☃`, Filename: `résumé"\.txt`, MediaType: "text/plain", Text: multipartText("{{cfg.secret}}")},
		{Name: "attachment", Filename: "/server/private/credentials.bin", BodyRaw: multipartText("AP+A")},
		{Name: "image", MediaType: "image/png", BodyRaw: multipartText("aW1hZ2U=")},
	}
	body, contentType, err := EncodeMultipart(context.Background(), parts, 4096)
	require.NoError(t, err)
	mediaType, parameters, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	require.NotEmpty(t, parameters["boundary"])
	require.NotContains(t, string(body), "filename*=")

	// Default text parts declare UTF-8 and preserve Unicode bytes; an explicit
	// text/plain type stays as authored without an added charset parameter.
	reader := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
	wantTypes := []string{"text/plain; charset=utf-8", "text/plain; charset=utf-8", "text/plain", "application/octet-stream", "image/png"}
	wantBodies := [][]byte{[]byte("Ünïcödé note\nline"), {}, []byte("{{cfg.secret}}"), {0, 255, 128}, []byte("image")}
	for i, source := range parts {
		part, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, source.Name, part.FormName())
		require.Equal(t, wantTypes[i], part.Header.Get("Content-Type"))
		_, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		require.NoError(t, err)
		require.Equal(t, source.Filename, disposition["filename"])
		payload, err := io.ReadAll(part)
		require.NoError(t, err)
		require.Equal(t, wantBodies[i], payload)
	}
	_, err = reader.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

// TestEncodeMultipartRejectsInvalidParts ensures validation covers every part
// and never leaks partial bytes or configuration values on failure.
func TestEncodeMultipartRejectsInvalidParts(t *testing.T) {
	for name, part := range map[string]tools.MultipartPart{
		"missing payload":  {Name: "private-config"},
		"two payloads":     {Name: "private-config", Text: multipartText("x"), BodyRaw: multipartText("eA==")},
		"empty name":       {Text: multipartText("private-config")},
		"blank name":       {Name: "  ", Text: multipartText("private-config")},
		"name injection":   {Name: "private-config\r\nX-Evil: yes", Text: multipartText("x")},
		"filename newline": {Name: "file", Filename: "private-config\nX-Evil: yes", BodyRaw: multipartText("eA==")},
		"filename nul":     {Name: "file", Filename: "private-config\x00", BodyRaw: multipartText("eA==")},
		"filename tab":     {Name: "file", Filename: "private-config\t", BodyRaw: multipartText("eA==")},
		"name DEL":         {Name: "private-config\x7f", Text: multipartText("x")},
		"name UTF-8":       {Name: string([]byte{0xff}), Text: multipartText("private-config")},
		"filename UTF-8":   {Name: "file", Filename: string([]byte{0xff}), Text: multipartText("private-config")},
		"text UTF-8":       {Name: "field", Text: multipartText(string([]byte{0xff}))},
		"bad base64":       {Name: "file", BodyRaw: multipartText("private-config")},
		"media injection":  {Name: "file", MediaType: "text/plain\r\nX-Evil: private-config", Text: multipartText("x")},
		"bare media":       {Name: "file", MediaType: "private-config", Text: multipartText("x")},
		"wildcard media":   {Name: "file", MediaType: "text/*", Text: multipartText("private-config")},
	} {
		t.Run(name, func(t *testing.T) {
			// A valid earlier part cannot escape when a later part is invalid.
			body, contentType, err := EncodeMultipart(context.Background(), []tools.MultipartPart{
				{Name: "first", Text: multipartText("valid")}, part,
			}, 4096)
			require.Error(t, err)
			require.Nil(t, body)
			require.Empty(t, contentType)
			require.NotContains(t, err.Error(), "private-config")
		})
	}
}

// TestEncodeMultipartBounds charges MIME framing in addition to payloads and
// validates aggregate source independently before binary decoding.
func TestEncodeMultipartBounds(t *testing.T) {
	// Boundary strings have stable length, so output size defines an exact cap.
	parts := []tools.MultipartPart{{Name: "field", Text: multipartText("value")}}
	body, _, err := EncodeMultipart(context.Background(), parts, 4096)
	require.NoError(t, err)
	encodedSize := len(body)
	body, contentType, err := EncodeMultipart(context.Background(), parts, encodedSize)
	require.NoError(t, err)
	require.Len(t, body, encodedSize)
	require.NotEmpty(t, contentType)

	// A one-byte-short cap must include the closing delimiter, not just content.
	body, contentType, err = EncodeMultipart(context.Background(), parts, encodedSize-1)
	require.ErrorContains(t, err, "encoded multipart byte limit")
	require.Nil(t, body)
	require.Empty(t, contentType)

	// Binary source can be larger than its eventual MIME body after decoding.
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, 3000))
	body, contentType, err = EncodeMultipart(context.Background(), []tools.MultipartPart{
		{Name: "file", BodyRaw: &encoded},
	}, 3500)
	require.ErrorContains(t, err, "source byte limit")
	require.Nil(t, body)
	require.Empty(t, contentType)

	// Many tiny or individually fitting parts share one source budget.
	for _, parts := range [][]tools.MultipartPart{
		{{Name: "a", Text: multipartText("")}, {Name: "b", Text: multipartText("")}},
		{{Name: "a", Text: multipartText("0123456789")}, {Name: "b", Text: multipartText("0123456789")}},
	} {
		body, contentType, err = EncodeMultipart(context.Background(), parts, len(parts[0].Name)+len(*parts[0].Text))
		require.ErrorContains(t, err, "source byte limit")
		require.Nil(t, body)
		require.Empty(t, contentType)
	}
}

// TestEncodeMultipartArgumentsAndCancellation rejects missing invocation
// context, disabled bounds, empty multipart lists, and canceled work.
func TestEncodeMultipartArgumentsAndCancellation(t *testing.T) {
	// All argument errors leave both outputs empty.
	for _, tc := range []struct {
		ctx   context.Context
		parts []tools.MultipartPart
		limit int
	}{
		{nil, nil, 4096},
		{context.Background(), nil, 4096},
		{context.Background(), nil, 0},
		{context.Background(), nil, -1},
	} {
		body, contentType, err := EncodeMultipart(tc.ctx, tc.parts, tc.limit)
		require.Error(t, err)
		require.Nil(t, body)
		require.Empty(t, contentType)
	}

	// Cancellation remains recognizable to the invocation layer.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body, contentType, err := EncodeMultipart(ctx, []tools.MultipartPart{{Name: "x", Text: multipartText("y")}}, 4096)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, body)
	require.Empty(t, contentType)

	// The writer also observes cancellation after encoding has begun.
	buffer := multipartBuffer{ctx: ctx, maxBytes: 4096}
	n, err := buffer.Write([]byte("private-config"))
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, n)
	require.Empty(t, buffer.Bytes())
}

// FuzzEncodeMultipart checks quoted metadata cannot change names or payloads
// when a successful body is decoded by the standard MIME reader.
func FuzzEncodeMultipart(f *testing.F) {
	f.Add("field", "file.txt", "value")
	f.Add(`a"\☃`, `résumé"\.txt`, "{{cfg.secret}}\r\n")
	f.Add("bad\r\nheader", "filename", "payload")
	f.Fuzz(func(t *testing.T, name, filename, text string) {
		// The inert codec either rejects the complete input or preserves it.
		const limit = 4096
		body, contentType, err := EncodeMultipart(context.Background(), []tools.MultipartPart{
			{Name: name, Filename: filename, Text: &text},
		}, limit)
		if err != nil {
			require.Nil(t, body)
			require.Empty(t, contentType)
			return
		}
		require.LessOrEqual(t, len(body), limit)
		_, parameters, err := mime.ParseMediaType(contentType)
		require.NoError(t, err)
		reader := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
		part, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, name, part.FormName())
		_, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		require.NoError(t, err)
		require.Equal(t, filename, disposition["filename"])
		payload, err := io.ReadAll(part)
		require.NoError(t, err)
		require.Equal(t, text, string(payload))
		_, err = reader.NextPart()
		require.ErrorIs(t, err, io.EOF)
	})
}
