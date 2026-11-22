package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestURLComponents exercises canonical HTTP URL components independently of
// the host's private parser representation and cached native strings.
func TestURLComponents(t *testing.T) {
	for _, test := range []struct {
		name, argument string
		want           []string
	}{
		{
			"credentials and default port", `"HTTPS://user:pass@EXAMPLE.test:443/a/../b?x=1&x=2#frag"`,
			[]string{"https://user:pass@example.test/b?x=1&x=2#frag", "https:", "user", "pass", "example.test", "example.test", "", "/b", "?x=1&x=2", "#frag", "https://example.test"},
		},
		{
			"HTTP empty path", `"http://EXAMPLE.test:80"`,
			[]string{"http://example.test/", "http:", "", "", "example.test", "example.test", "", "/", "", "", "http://example.test"},
		},
		{
			"IPv6 with nondefault port", `"https://[2001:0DB8::1]:8443/path"`,
			[]string{"https://[2001:db8::1]:8443/path", "https:", "", "", "[2001:db8::1]:8443", "[2001:db8::1]", "8443", "/path", "", "", "https://[2001:db8::1]:8443"},
		},
		{
			"IDNA and Unicode", `"https://bücher.example/café?q=雪#😀"`,
			[]string{"https://xn--bcher-kva.example/caf%C3%A9?q=%E9%9B%AA#%F0%9F%98%80", "https:", "", "", "xn--bcher-kva.example", "xn--bcher-kva.example", "", "/caf%C3%A9", "?q=%E9%9B%AA", "#%F0%9F%98%80", "https://xn--bcher-kva.example"},
		},
		{
			"IPv4 normalization", `"http://0x7f.1/"`,
			[]string{"http://127.0.0.1/", "http:", "", "", "127.0.0.1", "127.0.0.1", "", "/", "", "", "http://127.0.0.1"},
		},
		{
			"empty query and fragment markers", `"https://example.test/?#"`,
			[]string{"https://example.test/?#", "https:", "", "", "example.test", "example.test", "", "/", "", "", "https://example.test"},
		},
		{
			"surrogate replacement", `"https://example.test/\ud800?x=\udfff#\ud800"`,
			[]string{"https://example.test/%EF%BF%BD?x=%EF%BF%BD#%EF%BF%BD", "https:", "", "", "example.test", "example.test", "", "/%EF%BF%BD", "?x=%EF%BF%BD", "#%EF%BF%BD", "https://example.test"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Returning explicit fields avoids any implicit export of the branded URL.
			source := `(() => {const u = new URL(` + test.argument + `);
				return [u.href, u.protocol, u.username, u.password, u.host,
					u.hostname, u.port, u.pathname, u.search, u.hash, u.origin];})()`
			got, err := evaluateSource(context.Background(), runtimeLimits(), source)
			require.NoError(t, err)
			want, err := json.Marshal(test.want)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

// TestURLRelativeResolution keeps resolution and dot-segment normalization in
// the URL parser instead of treating references as string concatenation.
func TestURLRelativeResolution(t *testing.T) {
	for _, test := range []struct{ name, reference, want string }{
		{"parent path", "../c?q=2#new", "https://example.test/c?q=2#new"},
		{"absolute path", "/x", "https://example.test/x"},
		{"query only", "?q=2", "https://example.test/a/b?q=2"},
		{"fragment only", "#new", "https://example.test/a/b?old=1#new"},
		{"empty reference", "", "https://example.test/a/b?old=1"},
		{"network path", "//other.test:80/x", "https://other.test:80/x"},
		{"absolute URL", "http://different.test", "http://different.test/"},
		{"escaped dot segments", "/a/%2e%2e/b", "https://example.test/b"},
		{"spaces and Unicode", "../café snow", "https://example.test/caf%C3%A9%20snow"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Every reference resolves against the same copied primitive base.
			argument, err := json.Marshal(test.reference)
			require.NoError(t, err)
			source := `new URL(` + string(argument) + `, "https://example.test/a/b?old=1#old").href`
			got, err := evaluateSource(context.Background(), runtimeLimits(), source)
			require.NoError(t, err)
			want, err := json.Marshal(test.want)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

// TestURLSerialization returns the same canonical string through each explicit
// serializer while leaving query parsing to the existing independent helper.
func TestURLSerialization(t *testing.T) {
	source := `(() => {
		const u = new URL("HTTPS://example.test:443/a?q=a+b%2Bc#fragment", undefined);
		const q = new URLSearchParams(u.search);
		q.append("extra", "value");
		return [u.href, u.toString(), u.toJSON(), String(u), JSON.stringify(u),
			q.get("q"), q.toString(), u.search,
			typeof u.searchParams, typeof URL.parse, typeof URL.canParse,
			typeof URL.createObjectURL, typeof URL.revokeObjectURL];
	})()`

	// Query edits are detached, and APIs outside this immutable slice stay absent.
	got, err := evaluateSource(context.Background(), runtimeLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[
		"https://example.test/a?q=a+b%2Bc#fragment",
		"https://example.test/a?q=a+b%2Bc#fragment",
		"https://example.test/a?q=a+b%2Bc#fragment",
		"https://example.test/a?q=a+b%2Bc#fragment",
		"\"https://example.test/a?q=a+b%2Bc#fragment\"",
		"a b+c", "q=a+b%2Bc&extra=value", "?q=a+b%2Bc",
		"undefined", "undefined", "undefined", "undefined", "undefined"
	]`, string(got))
}

// TestURLRejectsInvalidInputs rejects unsupported schemes and malformed URLs
// with fixed safe text, and never executes authored string-conversion hooks.
func TestURLRejectsInvalidInputs(t *testing.T) {
	for _, expression := range []string{
		`new URL()`,
		`new URL(undefined)`,
		`new URL(null)`,
		`new URL(1)`,
		`new URL(true)`,
		`new URL(Symbol("privateDetail"))`,
		`new URL(new String("https://privateDetail.test/"))`,
		`new URL({toString() {touched++; return "https://privateDetail.test/";}})`,
		`new URL(new Proxy({}, {get() {touched++; throw "privateDetail";}}))`,
		`new URL("/relative")`,
		`new URL("")`,
		`new URL("https://privateDetail.test/", "")`,
		`new URL("https://privateDetail.test/", "not a base")`,
		`new URL("https://privateDetail.test/", null)`,
		`new URL("https://privateDetail.test/", {toString() {touched++; return "https://example.test/";}})`,
		`new URL("/relative", new URL("https://privateDetail.test/"))`,
		`new URL("https://")`,
		`new URL("https://privateDetail.test:65536/")`,
		`new URL("https://privateDetail.test:wrong/")`,
		`new URL("https://privateDetail:pass@%ZZ/")`,
		`new URL("https://[privateDetail]/")`,
		`new URL("file:///privateDetail")`,
		`new URL("ftp://privateDetail.test/")`,
		`new URL("data:text/plain,privateDetail")`,
		`new URL("javascript:privateDetail")`,
		`new URL("https://example.test/", "file:///privateDetail")`,
	} {
		t.Run(expression, func(t *testing.T) {
			// Invalid input must not expose parser diagnostics, credentials, or raw URLs.
			source := `(() => {let touched = 0;
				try {` + expression + `; return "accepted";} catch (error) {
					return [error instanceof TypeError, touched, error.message.includes("privateDetail")];
				}})()`
			got, err := evaluateSource(context.Background(), runtimeLimits(), source)
			require.NoError(t, err)
			require.JSONEq(t, `[true,0,false]`, string(got))
		})
	}
}

// TestURLConstructorAndReceiverBrands requires genuine construction without
// granting the native state to prototype lookalikes or forwarding proxies.
func TestURLConstructorAndReceiverBrands(t *testing.T) {
	source := `(() => {
		let touched = 0, rejected = 0;
		const fake = Object.create(URL.prototype), real = new URL("https://example.test/");
		try {URL("https://privateDetail.test/");} catch (error) {if (error instanceof TypeError) rejected++;}
		try {URL.call(fake, "https://privateDetail.test/");} catch (error) {if (error instanceof TypeError) rejected++;}
		const proxy = new Proxy(real, {get() {touched++; throw "privateDetail";}});
		const revoked = Proxy.revocable(real, {}); revoked.revoke();
		const properties = ["href", "protocol", "username", "password", "host", "hostname",
			"port", "pathname", "search", "hash", "origin"];
		for (const value of [fake, {}, proxy, revoked.proxy]) {
			for (const name of properties) {
				try {Object.getOwnPropertyDescriptor(URL.prototype, name).get.call(value);}
				catch (error) {if (error instanceof TypeError) rejected++;}
			}
			for (const name of ["toString", "toJSON"]) {
				try {URL.prototype[name].call(value);}
				catch (error) {if (error instanceof TypeError) rejected++;}
			}
		}
		class Derived extends URL {}
		const derived = new Derived("../derived", "https://example.test/a/b");
		function Alternate() {}
		const alternate = Reflect.construct(URL, ["https://example.test/alternate"], Alternate);
		const href = Object.getOwnPropertyDescriptor(URL.prototype, "href").get;
		return [rejected, touched, derived instanceof Derived, derived instanceof URL,
			derived.href, Object.getPrototypeOf(alternate) === Alternate.prototype,
			href.call(alternate), URL.prototype.toJSON.call(alternate), URL.prototype.toString.call(real)];
	})()`

	// Borrowing a method is valid only for branded objects created in this VM.
	got, err := evaluateSource(context.Background(), runtimeLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[54,0,true,true,"https://example.test/derived",true,
		"https://example.test/alternate","https://example.test/alternate","https://example.test/"]`, string(got))
}

// TestURLProtectedSurface keeps the global and prototype contract stable and
// exposes parsed components through getters without mutation setters.
func TestURLProtectedSurface(t *testing.T) {
	for _, name := range []string{"href", "protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash", "origin"} {
		t.Run(name, func(t *testing.T) {
			// Strict assignment fails without changing the cached native component.
			source := fmt.Sprintf(`(() => {"use strict";
				const u = new URL("https://example.test/a?q=1#f"), before = u[%q];
				const d = Object.getOwnPropertyDescriptor(URL.prototype, %q);
				let rejected = false;
				try {u[%q] = "privateDetail";} catch (error) {rejected = error instanceof TypeError;}
				return [rejected, u[%q] === before, typeof d.get, typeof d.set, d.enumerable, d.configurable];
			})()`, name, name, name, name)
			got, err := evaluateSource(context.Background(), runtimeLimits(), source)
			require.NoError(t, err)
			require.JSONEq(t, `[true,true,"function","undefined",false,false]`, string(got))
		})
	}

	// Protected constructor/method properties cannot be replaced by authored code.
	source := `(() => {"use strict";
		let rejected = 0;
		const attempts = [
			() => {globalThis.URL = null;},
			() => {URL.prototype = {};},
			() => {URL.prototype.toString = () => "privateDetail";},
			() => {URL.prototype.toJSON = () => "privateDetail";},
			() => Object.defineProperty(URL.prototype, "href", {get() {return "privateDetail";}})
		];
		for (const attempt of attempts) {try {attempt();} catch (error) {if (error instanceof TypeError) rejected++;}}
		const descriptor = Object.getOwnPropertyDescriptor(globalThis, "URL");
		return [rejected, descriptor.writable, descriptor.configurable, descriptor.enumerable,
			new URL("https://example.test/").toString()];
	})()`
	got, err := evaluateSource(context.Background(), runtimeLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[5,false,false,false,"https://example.test/"]`, string(got))
}
