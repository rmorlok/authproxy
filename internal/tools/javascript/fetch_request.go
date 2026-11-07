package javascript

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"golang.org/x/net/http/httpguts"
)

// prepareFetchRequest copies one fetch call into a detached, replayable request.
// The URL must be a primitive absolute HTTP(S) string. The optional init object
// supports only own method, headers, and body properties; headers use own
// enumerable string properties, and body supports strings or native ArrayBuffers.
// Methods follow Fetch normalization and its CONNECT/TRACE/TRACK exclusions.
// JSON must be explicitly serialized by authored code. No credentials are added:
// the connection's HTTP client retains final authentication precedence.
//
// All VM access, including getters and Proxy traps, stays on the calling owner
// goroutine. The host callback must use vm.Try to catch authored exceptions.
// Returned errors contain no URL, header values, body, or authored diagnostics.
func prepareFetchRequest(
	ctx context.Context,
	vm *goja.Runtime,
	call goja.FunctionCall,
	maxBytes int,
) (*http.Request, error) {
	if ctx == nil || vm == nil || maxBytes <= 0 || len(call.Arguments) > 2 {
		return nil, apjsruntime.ErrInvalidInput
	}

	budget := fetchRequestBudget{
		ctx:       ctx,
		remaining: maxBytes,
	}
	rawURL, err := budget.string(call.Argument(0))
	if err != nil {
		return nil, err
	}

	if !validFetchURL(rawURL) {
		return nil, apjsruntime.ErrInvalidInput
	}

	methodValue := vm.ToValue("GET")
	var headersValue, bodyValue goja.Value

	if initValue := call.Argument(1); !goja.IsUndefined(initValue) && !goja.IsNull(initValue) {
		init, ok := initValue.(*goja.Object)
		if !ok || init.ClassName() != "Object" || len(init.Symbols()) != 0 {
			return nil, apjsruntime.ErrInvalidInput
		}

		// Validate all names before evaluating getters so unsupported options
		// cannot silently acquire a different meaning from browser fetch.
		keys := init.GetOwnPropertyNames()
		for _, key := range keys {
			if key != "method" && key != "headers" && key != "body" {
				return nil, apjsruntime.ErrInvalidInput
			}
		}

		for _, key := range keys {
			value := init.Get(key)
			switch key {
			case "method":
				if !goja.IsUndefined(value) {
					methodValue = value
				}
			case "headers":
				headersValue = value
			case "body":
				bodyValue = value
			}
		}
	}

	method, err := budget.string(methodValue)
	if err != nil {
		return nil, err
	}

	if !tools.IsHTTPToken(method) {
		return nil, apjsruntime.ErrInvalidInput
	}

	// Fetch uppercases only these six methods; other valid tokens remain
	// case-sensitive, including PATCH. Token validation makes this ASCII-only.
	switch upper := strings.ToUpper(method); upper {
	case "CONNECT", "TRACE", "TRACK":
		return nil, apjsruntime.ErrInvalidInput
	case "DELETE", "GET", "HEAD", "OPTIONS", "POST", "PUT":
		method = upper
	}

	headers, host, err := budget.headers(headersValue)
	if err != nil {
		return nil, err
	}

	body, err := budget.body(bodyValue)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, apjsruntime.ErrInvalidInput
	}

	request.Header = headers
	if host != "" {
		request.Host = host
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return request, nil
}

// fetchRequestBudget counts the prepared method, URL, header names/values, and
// body together. It is not a wire-size limit or a JavaScript heap quota.
type fetchRequestBudget struct {
	ctx       context.Context // Invocation deadline remains active through conversion.
	remaining int             // Bytes still available before another detached copy.
}

// consume reserves bytes without overflow and preserves caller cancellation.
func (b *fetchRequestBudget) consume(size int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}

	if size > b.remaining {
		return apjsruntime.ErrLimitExceeded
	}

	b.remaining -= size

	return nil
}

// string checks the UTF-16 length before conversion, then accounts exact UTF-8
// bytes before allocating the Go string. Lone surrogates are rejected instead of
// silently changing private request data to replacement characters.
func (b *fetchRequestBudget) string(value goja.Value) (string, error) {
	s, ok := value.(goja.String)
	if !ok {
		return "", apjsruntime.ErrInvalidInput
	}

	if s.Length() > b.remaining {
		return "", apjsruntime.ErrLimitExceeded
	}

	size := 0
	for i := 0; i < s.Length(); i++ {
		if err := b.ctx.Err(); err != nil {
			return "", err
		}
		unit := s.CharAt(i)
		switch {
		case unit < utf8.RuneSelf:
			size++
		case unit < 0x800:
			size += 2
		case unit >= 0xd800 && unit <= 0xdbff:
			i++
			if i >= s.Length() || s.CharAt(i) < 0xdc00 || s.CharAt(i) > 0xdfff {
				return "", apjsruntime.ErrInvalidInput
			}
			size += 4
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", apjsruntime.ErrInvalidInput
		default:
			size += 3
		}

		if size > b.remaining {
			return "", apjsruntime.ErrLimitExceeded
		}
	}

	if err := b.consume(size); err != nil {
		return "", err
	}

	return s.String(), nil
}

// headers copies a string record into canonical headers and a separate Host
// override. Framing and hop-by-hop fields remain owned by the transport.
func (b *fetchRequestBudget) headers(value goja.Value) (http.Header, string, error) {
	headers := make(http.Header)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return headers, "", nil
	}

	object, ok := value.(*goja.Object)
	if !ok || object.ClassName() != "Object" || len(object.Symbols()) != 0 {
		return nil, "", apjsruntime.ErrInvalidInput
	}

	var host string
	seen := make(map[string]bool)

	for _, name := range object.Keys() {
		if err := b.consume(len(name)); err != nil {
			return nil, "", err
		}

		if !tools.IsHTTPToken(name) {
			return nil, "", apjsruntime.ErrInvalidInput
		}

		// Make sure header is capitalized in the standard way
		canonical := http.CanonicalHeaderKey(name)

		// Can't have the same header twice
		if seen[canonical] {
			return nil, "", apjsruntime.ErrInvalidInput
		}
		seen[canonical] = true

		switch canonical {
		case "Content-Length",
			"Transfer-Encoding",
			"Trailer",
			"Trailers",
			"Connection",
			"Keep-Alive",
			"Proxy-Authenticate",
			"Proxy-Authorization",
			"Te",
			"Upgrade":
			return nil, "", apjsruntime.ErrInvalidInput
		}

		text, err := b.string(object.Get(name))
		if err != nil {
			return nil, "", err
		}

		if !httpguts.ValidHeaderFieldValue(text) {
			return nil, "", apjsruntime.ErrInvalidInput
		}

		if canonical == "Host" {
			if !validFetchAuthority(text) {
				return nil, "", apjsruntime.ErrInvalidInput
			}
			host = text
		} else {
			headers.Set(canonical, text)
		}
	}

	return headers, host, nil
}

// body constructs a replayable reader after bounding its data. ArrayBuffer's
// exported bytes alias the VM, so they are cloned before a worker can use them.
// Typed arrays are intentionally unsupported; callers may pass an ArrayBuffer.
func (b *fetchRequestBudget) body(value goja.Value) (io.Reader, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}

	if _, ok := value.(goja.String); ok {
		text, err := b.string(value)
		if err != nil {
			return nil, err
		}

		return strings.NewReader(text), nil
	}

	if value.ExportType() != reflect.TypeFor[goja.ArrayBuffer]() {
		return nil, apjsruntime.ErrInvalidInput
	}

	buffer := value.Export().(goja.ArrayBuffer)
	if buffer.Detached() {
		return nil, apjsruntime.ErrInvalidInput
	}

	if err := b.consume(len(buffer.Bytes())); err != nil {
		return nil, err
	}

	return bytes.NewReader(bytes.Clone(buffer.Bytes())), nil
}

// validFetchURL limits the URL to an absolute HTTP(S) destination without
// fragments or unescaped ASCII whitespace/controls. Parsing and authority
// validation do not resolve DNS or impose a destination allowlist.
func validFetchURL(raw string) bool {
	for i := range len(raw) {
		if raw[i] <= ' ' || raw[i] == 0x7f || raw[i] == '#' {
			return false
		}
	}

	u, err := url.Parse(raw)
	return err == nil &&
		(u.Scheme == "http" || u.Scheme == "https") &&
		u.Opaque == "" &&
		validFetchAuthority(u.Host)
}

// validFetchAuthority applies net/url and net/http host validation to both
// destination authorities and Host overrides. It excludes other URL components
// and invalid port ranges while preserving the original authority spelling.
func validFetchAuthority(authority string) bool {
	if authority == "" || strings.ContainsAny(authority, "/\\?#@") {
		return false
	}

	u, err := url.Parse("http://" + authority)
	if err != nil || u.Host != authority || u.Hostname() == "" {
		return false
	}

	if port := u.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return false
		}
	}

	ascii, err := httpguts.PunycodeHostPort(authority)
	return err == nil && httpguts.ValidHostHeader(ascii)
}
