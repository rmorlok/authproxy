package runtime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// fetchRequestCall builds arguments inside the VM without exposing Go objects.
func fetchRequestCall(t *testing.T, vm *goja.Runtime, source string) goja.FunctionCall {
	t.Helper()
	value, err := vm.RunString("[" + source + "]")
	require.NoError(t, err)
	array := value.ToObject(vm)
	call := goja.FunctionCall{}
	for i := int64(0); i < array.Get("length").ToInteger(); i++ {
		call.Arguments = append(call.Arguments, array.Get(vm.ToValue(i).String()))
	}
	return call
}

// TestPrepareFetchRequestConstruction preserves authored HTTP data and replay semantics.
func TestPrepareFetchRequestConstruction(t *testing.T) {
	vm := goja.New()
	call := fetchRequestCall(t, vm, `"https://network.example.test:8443/v1?q=one&q=two", {
		method: "POST", headers: {
			Host: "tenant.example.test", "content-type": "application/json",
			Authorization: "authored credential", "X-Example": "one\ttwo"
		}, body: JSON.stringify({name: "Ada"})
	}`)

	// Host routing is separate; connection authentication may later replace the authored candidate.
	request, err := prepareFetchRequest(context.Background(), vm, call, 1024)
	require.NoError(t, err)
	require.Equal(t, "POST", request.Method)
	require.Equal(t, "network.example.test:8443", request.URL.Host)
	require.Equal(t, "q=one&q=two", request.URL.RawQuery)
	require.Equal(t, "tenant.example.test", request.Host)
	require.Empty(t, request.Header.Get("Host"))
	require.Equal(t, "authored credential", request.Header.Get("Authorization"))
	require.Equal(t, "application/json", request.Header.Get("Content-Type"))
	require.Equal(t, "one\ttwo", request.Header.Get("X-Example"))

	// Both the initial body and its replay retain the same serialized bytes.
	for range 2 {
		body, err := request.GetBody()
		require.NoError(t, err)
		data, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		require.Equal(t, `{"name":"Ada"}`, string(data))
	}
}

// TestPrepareFetchRequestArrayBufferOwnsBytes prevents VM mutation from changing a dispatched request.
func TestPrepareFetchRequestArrayBufferOwnsBytes(t *testing.T) {
	vm := goja.New()
	_, err := vm.RunString(`var bytes = new Uint8Array([1, 2, 255]);`)
	require.NoError(t, err)
	call := fetchRequestCall(t, vm, `"https://example.test", {method: "PUT", body: bytes.buffer}`)
	request, err := prepareFetchRequest(context.Background(), vm, call, 1024)
	require.NoError(t, err)

	// Mutate the original buffer after preparation, then verify first send and replay.
	_, err = vm.RunString(`bytes.fill(9)`)
	require.NoError(t, err)
	first, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.NoError(t, request.Body.Close())
	replay, err := request.GetBody()
	require.NoError(t, err)
	second, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, []byte{1, 2, 255}, first)
	require.Equal(t, first, second)
}

// TestPrepareFetchRequestRecordSemantics only reads own properties and supports omitted data.
func TestPrepareFetchRequestRecordSemantics(t *testing.T) {
	for _, source := range []string{
		`"https://example.test"`,
		`"https://example.test", null`,
		`"https://example.test", {method: undefined, body: null, headers: null}`,
		`"https://example.test", Object.create({method: "POST", headers: {"X-Inherited": "secret"}})`,
		`"https://example.test", {headers: Object.create({"X-Inherited": "secret"})}`,
	} {
		// Missing and inherited input leave the ordinary GET request untouched.
		vm := goja.New()
		request, err := prepareFetchRequest(context.Background(), vm, fetchRequestCall(t, vm, source), 1024)
		require.NoError(t, err, source)
		require.Equal(t, "GET", request.Method)
		require.Empty(t, request.Header)
		require.Nil(t, request.Body)
	}
}

// TestPrepareFetchRequestMethodNormalization applies Fetch's six-method rule while preserving other tokens.
func TestPrepareFetchRequestMethodNormalization(t *testing.T) {
	for _, test := range []struct {
		authored string
		want     string
	}{
		{"delete", "DELETE"},
		{"DeLeTe", "DELETE"},
		{"get", "GET"},
		{"GeT", "GET"},
		{"head", "HEAD"},
		{"HeAd", "HEAD"},
		{"options", "OPTIONS"},
		{"OpTiOnS", "OPTIONS"},
		{"post", "POST"},
		{"PoSt", "POST"},
		{"put", "PUT"},
		{"PuT", "PUT"},
		{"PATCH", "PATCH"},
		{"patch", "patch"},
		{"PaTcH", "PaTcH"},
		{"Custom-Method", "Custom-Method"},
	} {
		t.Run(test.authored, func(t *testing.T) {
			vm := goja.New()
			call := fetchRequestCall(t, vm, `"https://example.test", {method: "`+test.authored+`"}`)

			// Normalization changes the transmitted method, without changing its byte allowance.
			request, err := prepareFetchRequest(context.Background(), vm, call, len("https://example.test")+len(test.authored))
			require.NoError(t, err)
			require.Equal(t, test.want, request.Method)
		})
	}
}

// TestPrepareFetchRequestForbiddenMethods rejects each Fetch-forbidden method regardless of case.
func TestPrepareFetchRequestForbiddenMethods(t *testing.T) {
	for _, method := range []string{
		"CONNECT", "connect", "CoNnEcT",
		"TRACE", "trace", "TrAcE",
		"TRACK", "track", "TrAcK",
	} {
		t.Run(method, func(t *testing.T) {
			vm := goja.New()
			call := fetchRequestCall(t, vm, `"https://private.example.test", {
				method: "`+method+`", headers: {get "X-Private"() {throw "private";}}
			}`)

			// Reject before copying header values or returning a request that could be dispatched.
			request, err := prepareFetchRequest(context.Background(), vm, call, 1024)
			require.ErrorIs(t, err, ErrInvalidInput)
			require.NotContains(t, err.Error(), "private")
			require.Nil(t, request)
		})
	}
}

// TestPrepareFetchRequestBounds charges all prepared fields and UTF-8 bytes together.
func TestPrepareFetchRequestBounds(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		size   int
	}{
		{"default method", `"https://example.test"`, len("https://example.testGET")},
		{"all fields", `"https://example.test", {method: "POST", headers: {Host: "tenant.test", "X-Data": "value"}, body: "hi"}`,
			len("https://example.testPOSTHosttenant.testX-Datavaluehi")},
		{"UTF8 text", `"https://example.test", {body: "é😀"}`, len("https://example.testGETé😀")},
		{"buffer", `"https://example.test", {body: new Uint8Array([0, 1, 2]).buffer}`, len("https://example.testGET") + 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			vm := goja.New()
			call := fetchRequestCall(t, vm, test.source)
			// The exact shared allowance succeeds; one fewer byte returns no partial request.
			request, err := prepareFetchRequest(context.Background(), vm, call, test.size)
			require.NoError(t, err)
			require.NotNil(t, request)
			request, err = prepareFetchRequest(context.Background(), vm, call, test.size-1)
			require.ErrorIs(t, err, ErrLimitExceeded)
			require.Nil(t, request)
		})
	}
}

// TestPrepareFetchRequestRejectsUnsupportedData fails safely without retaining private arguments.
func TestPrepareFetchRequestRejectsUnsupportedData(t *testing.T) {
	for _, source := range []string{
		`{toString() { throw "private"; }}`,
		`"/private"`, `"file:///private"`, `"https:///private"`, `"https://private/path#fragment"`,
		`"https://private/path?q=two words"`, `"https://private:65536"`,
		`"https://private", {method: "POST\r\nprivate"}`, `"https://private", {method: null}`,
		`"https://private", {redirect: "manual"}`, `"https://private", {signal: {}}`,
		`"https://private", [], "extra"`, `"https://private", []`,
		`"https://private", {headers: [["X-Private", "value"]]}`,
		`"https://private", {headers: {"X-Private": 123}}`,
		`"https://private", {headers: {"X-Private": "first", "x-private": "second"}}`,
		`"https://private", {headers: {"X-Private": "value\r\nInjected: yes"}}`,
		`"https://private", {headers: {Host: "private/path"}}`,
		`"https://private", {body: {private: true}}`, `"https://private", {body: [1, 2]}`,
		`"https://private", {body: new Uint8Array([1, 2])}`, `"https://private", {body: "\ud800"}`,
	} {
		// No unsupported value is coerced or included in the public classification.
		vm := goja.New()
		request, err := prepareFetchRequest(context.Background(), vm, fetchRequestCall(t, vm, source), 1024)
		require.ErrorIs(t, err, ErrInvalidInput, source)
		require.NotContains(t, err.Error(), "private")
		require.Nil(t, request)
	}
	for _, name := range []string{"Content-Length", "Transfer-Encoding", "Trailer", "Trailers", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Upgrade"} {
		// Transport-controlled headers are rejected regardless of capitalization.
		vm := goja.New()
		source := `"https://example.test", {headers: {"` + strings.ToLower(name) + `": "private"}}`
		request, err := prepareFetchRequest(context.Background(), vm, fetchRequestCall(t, vm, source), 1024)
		require.ErrorIs(t, err, ErrInvalidInput, name)
		require.Nil(t, request)
	}
}

// TestPrepareFetchRequestCancellationAndGetters preserves cancellation and leaves VM exceptions to its owner.
func TestPrepareFetchRequestCancellationAndGetters(t *testing.T) {
	vm := goja.New()
	call := fetchRequestCall(t, vm, `"https://example.test", {headers: {get "X-Value"() {throw "private";}}}`)
	var request *http.Request
	// Getter exceptions are intercepted by the host's VM boundary, before HTTP dispatch.
	exception := vm.Try(func() {
		request, _ = prepareFetchRequest(context.Background(), vm, call, 1024)
	})
	require.NotNil(t, exception)
	require.Nil(t, request)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation wins while copying a valid request, with no request returned.
	request, err := prepareFetchRequest(ctx, vm, fetchRequestCall(t, vm, `"https://example.test"`), 1024)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, request)
}
