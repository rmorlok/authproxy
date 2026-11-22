package runtime

import (
	"github.com/dop251/goja"
	whatwgurl "github.com/nlnwa/whatwg-url/url"
)

// argument admits only primitive strings and converts UTF-16 to scalar UTF-8,
// replacing lone surrogates. Existing codec loops poll caller cancellation;
// objects are rejected without reading properties or invoking coercion.
func (u *urlHost) argument(value goja.Value) string {
	text, ok := value.(goja.String)
	if !ok {
		u.invalidInput()
	}
	u.host.consumeInput(text.Length())
	encoded := u.host.encodeUTF8(text)
	u.host.consumeInput(len(encoded))
	u.host.reserveOutput(len(encoded))
	result := string(encoded)
	u.host.checkContext()
	return result
}

// admitNative reserves a conservative quadratic allowance before a parser or
// serialization pass. The pinned Go-compatible parser can repeatedly copy
// userinfo and path prefixes; a linear charge would miss that temporary work
// and storage. Logical units bound those costs by fixed implementation factors.
// Resolution includes the base's serialized length because parsing clones it.
// Native parser calls have no cancellation hook, so their admitted input bounds
// the work between the surrounding context checks. No goroutine retains the VM.
func (u *urlHost) admitNative(size int) {
	u.host.checkContext()
	size = max(size, 1)
	if size > int(^uint(0)>>1)/size {
		u.host.failLimit()
	}
	units := size * size
	u.host.consumeInput(units)
	u.host.reserveOutput(units)
}

// checkParsed preserves cancellation before reporting fixed malformed-input
// errors. This slice permits HTTP(S) URLs only, including supplied bases; the
// parser's full set of schemes does not grant extra runtime capabilities.
func (u *urlHost) checkParsed(parsed *whatwgurl.Url, err error) {
	u.host.checkContext()
	if err != nil || parsed == nil || (parsed.Scheme() != "http" && parsed.Scheme() != "https") {
		u.invalidInput()
	}
}
