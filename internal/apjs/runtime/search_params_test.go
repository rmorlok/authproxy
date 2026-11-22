package runtime_test

import (
	"context"
	"fmt"
	"testing"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// searchParamsLimits provides room for ordinary fixtures.
func searchParamsLimits() jsruntime.Limits { return runtimeLimits() }

// TestSearchParamsFormParsing preserves tuple order and applies form encoding
// rather than interpreting the input as a URL or a general URI component.
func TestSearchParamsFormParsing(t *testing.T) {
	for _, test := range []struct {
		name, argument, serialized string
		size                       int
	}{
		{"omitted", "", "", 0},
		{"undefined", "undefined", "", 0},
		{"empty", `""`, "", 0},
		{"empty query", `"?&&"`, "", 0},
		{"one question mark", `"??a=1"`, "%3Fa=1", 1},
		{"empty segments", `"&&=&foo&&bar=&a=b=c&"`, "=&foo=&bar=&a=b%3Dc", 4},
		{"duplicate order", `"?x=1&z=2&x=3"`, "x=1&z=2&x=3", 3},
		{"property-like names", `"__proto__=x&constructor=y&toString=z"`, "__proto__=x&constructor=y&toString=z", 3},
		{"space and plus", `"q=a+b%2Bc%20d"`, "q=a+b%2Bc+d", 1},
		{"encoded separators", `"%26%3D=%3d%26%3f%23"`, "%26%3D=%3D%26%3F%23", 1},
		{"invalid escapes", `"x=%&y=%2&z=%GG"`, "x=%25&y=%252&z=%25GG", 3},
		{"fragment is data", `"x=1#fragment"`, "x=1%23fragment", 1},
		{"URL is data", `"https://example.test/?x=1"`, "https%3A%2F%2Fexample.test%2F%3Fx=1", 1},
		{"Unicode", `"é=雪&emoji=😀"`, "%C3%A9=%E9%9B%AA&emoji=%F0%9F%98%80", 2},
		{"lone surrogate", `"x=\ud800"`, "x=%EF%BF%BD", 1},
		{"BOM retained", `"%EF%BB%BFx=%EF%BB%BFy"`, "%EF%BB%BFx=%EF%BB%BFy", 1},
		{"NUL retained", `"x=%00"`, "x=%00", 1},
		{"truncated UTF8", `"x=%E2%82"`, "x=%EF%BF%BD", 1},
		{"invalid continuation", `"x=%E2%28%A1"`, "x=%EF%BF%BD%28%EF%BF%BD", 1},
		{"overlong UTF8", `"x=%C0%80"`, "x=%EF%BF%BD%EF%BF%BD", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Independent serialized fixtures cover normalization as well as tuple counts.
			source := fmt.Sprintf(`(() => {const p = new URLSearchParams(%s);
				return [p.size, p.toString()];})()`, test.argument)
			got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`[%d,%q]`, test.size, test.serialized), string(got))
		})
	}
}

// TestSearchParamsMutation exercises the ordered-list behavior of each mutator
// and the difference between absent names and names with empty values.
func TestSearchParamsMutation(t *testing.T) {
	source := `(() => {
		const p = new URLSearchParams("first=0&x=1&middle=2&x=3&last=4&empty");
		const initial = [p.get("x"), p.getAll("x"), p.get("missing"), p.get("empty"),
			p.has("missing"), p.has("empty"), p.has("x", "3"), p.has("x", "4"),
			p.has("x", undefined), p.getAll("missing")];
		const returns = [typeof p.set("x", "new"), typeof p.append("x", "tail")];
		const replaced = [p.size, p.toString(), p.getAll("x")];
		returns.push(typeof p.delete("x", "new"));
		const selected = [p.toString(), p.get("x")];
		p.delete("x", undefined);
		p.delete("missing");
		p.set("added", "5");
		p.append("", "");
		return [initial, returns, replaced, selected, p.size, p.toString()];
	})()`

	// Set retains the first matching position; append and a new set add at the end.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[["1",["1","3"],null,"",false,true,true,false,true,[]],
		["undefined","undefined","undefined"],
		[6,"first=0&x=new&middle=2&last=4&empty=&x=tail",["new","tail"]],
		["first=0&middle=2&last=4&empty=&x=tail","tail"],
		6,"first=0&middle=2&last=4&empty=&added=5&="]`, string(got))
}

// TestSearchParamsValueMatching removes every matching tuple while keeping
// same-name tuples with other values and differently named tuples in place.
func TestSearchParamsValueMatching(t *testing.T) {
	source := `(() => {
		const p = new URLSearchParams("x=1&other=1&x=2&x=1&x=&x=undefined");
		p.delete("x", "1");
		const matched = [p.has("x", "1"), p.has("x", "2"), p.has("x", ""), p.toString()];
		p.delete("x", "");
		p.delete("x", "undefined");
		const remaining = p.toString();
		p.delete("x");
		return [matched, remaining, p.toString(), p.size];
	})()`

	// An explicit string value filters deletion; an omitted value removes the name entirely.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[[false,true,true,"other=1&x=2&x=&x=undefined"],"other=1&x=2","other=1",1]`, string(got))
}

// TestSearchParamsScalarStrings normalizes method arguments without treating
// their percent signs or plus characters as already-encoded query syntax.
func TestSearchParamsScalarStrings(t *testing.T) {
	source := `(() => {
		const p = new URLSearchParams();
		p.append("\ud800", "\udfff");
		p.append("AZaz09*-._ =~!()'/?#&+", "é😀\u0000");
		p.append("%2B", "+");
		const before = [p.get("\ufffd"), p.get("\udfff"), p.has("\ud800", "\ud800"), p.toString()];
		p.set("\udfff", "updated");
		p.delete("\ud800", "updated");
		return [before, p.size, p.has("\ufffd")];
	})()`

	// Both names and values use scalar strings, and only the form-safe ASCII set remains literal.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[["\ufffd","\ufffd",true,
		"%EF%BF%BD=%EF%BF%BD&AZaz09*-._+%3D%7E%21%28%29%27%2F%3F%23%26%2B=%C3%A9%F0%9F%98%80%00&%252B=%2B"],2,false]`, string(got))
}

// TestSearchParamsStableSort uses UTF-16 name order and preserves the order of
// values attached to equal names, including when their values differ.
func TestSearchParamsStableSort(t *testing.T) {
	source := `(() => {
		const p = new URLSearchParams("\ue000=private&😀=supplementary&a=first&\ufffd=replacement&a=second&A=upper");
		const result = p.sort();
		const once = p.toString();
		p.sort();
		return [typeof result, once, p.toString(), p.getAll("a")];
	})()`

	// UTF-16 puts the supplementary name before U+E000, unlike UTF-8 or scalar ordering.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	want := `A=upper&a=first&a=second&%F0%9F%98%80=supplementary&%EE%80%80=private&%EF%BF%BD=replacement`
	require.JSONEq(t, fmt.Sprintf(`["undefined",%q,%q,["first","second"]]`, want, want), string(got))
}

// TestSearchParamsRejectsCoercion keeps the supported constructor and method
// contract limited to primitive strings without executing authored getters.
func TestSearchParamsRejectsCoercion(t *testing.T) {
	for _, source := range []string{
		`new URLSearchParams(null)`,
		`new URLSearchParams(1)`,
		`new URLSearchParams(true)`,
		`new URLSearchParams(new String("x=privateDetail"))`,
		`new URLSearchParams([["x", "privateDetail"]])`,
		`new URLSearchParams({get x() {touched++; return "privateDetail";}})`,
		`new URLSearchParams({toString() {touched++; return "privateDetail";}})`,
		`new URLSearchParams(new Proxy({}, {get() {touched++; throw "privateDetail";}}))`,
		`new URLSearchParams(new URLSearchParams("x=1"))`,
		`p.append("x")`,
		`p.append("x", undefined)`,
		`p.append("x", {toString() {touched++; return "privateDetail";}})`,
		`p.set("x", null)`,
		`p.set({}, "x")`,
		`p.get()`,
		`p.get(Symbol("privateDetail"))`,
		`p.getAll(new String("x"))`,
		`p.has(undefined)`,
		`p.has("x", 1)`,
		`p.delete("x", null)`,
		`p.delete({get toString() {touched++; throw "privateDetail";}})`,
	} {
		t.Run(source, func(t *testing.T) {
			// Failed validation must neither reveal the supplied value nor run its conversion hooks.
			wrapped := `(() => {let touched = 0; const p = new URLSearchParams("x=1");
				try {` + source + `; return "accepted";} catch (error) {
				return [error instanceof TypeError, touched, error.message.includes("privateDetail"), p.toString()];}})()`
			got, err := evaluateSource(context.Background(), searchParamsLimits(), wrapped)
			require.NoError(t, err)
			require.JSONEq(t, `[true,0,false,"x=1"]`, string(got))
		})
	}
}

// TestSearchParamsConstructorAndBrands requires construction and authentic
// receivers while allowing subclass instances and borrowed genuine methods.
func TestSearchParamsConstructorAndBrands(t *testing.T) {
	source := `(() => {
		let touched = 0, rejected = 0;
		const fake = Object.create(URLSearchParams.prototype);
		try {URLSearchParams("x=1");} catch (error) {if (error instanceof TypeError) rejected++;}
		try {URLSearchParams.call(fake, "x=1");} catch (error) {if (error instanceof TypeError) rejected++;}
		const real = new URLSearchParams("x=real");
		const proxy = new Proxy(real, {get() {touched++; throw "privateDetail";}});
		const revoked = Proxy.revocable(real, {}); revoked.revoke();
		const size = Object.getOwnPropertyDescriptor(URLSearchParams.prototype, "size").get;
		const methods = [
			[p => URLSearchParams.prototype.append.call(p, "x", "1")],
			[p => URLSearchParams.prototype.set.call(p, "x", "1")],
			[p => URLSearchParams.prototype.get.call(p, "x")],
			[p => URLSearchParams.prototype.getAll.call(p, "x")],
			[p => URLSearchParams.prototype.has.call(p, "x")],
			[p => URLSearchParams.prototype.delete.call(p, "x")],
			[p => URLSearchParams.prototype.sort.call(p)],
			[p => URLSearchParams.prototype.toString.call(p)],
			[p => size.call(p)]
		];
		for (const value of [fake, {}, proxy, revoked.proxy]) {
			for (const [method] of methods) {
				try {method(value);} catch (error) {if (error instanceof TypeError) rejected++;}
			}
		}
		class Derived extends URLSearchParams {}
		const derived = new Derived("x=derived");
		function Alternate() {}
		const alternate = Reflect.construct(URLSearchParams, ["x=alternate"], Alternate);
		return [rejected, touched, real.get("x"), real.get.call(derived, "x"),
			derived instanceof Derived, derived instanceof URLSearchParams,
			Object.getPrototypeOf(alternate) === Alternate.prototype,
			URLSearchParams.prototype.get.call(alternate, "x"), size.call(alternate)];
	})()`

	// Prototype resemblance and proxy forwarding cannot acquire the private query-state brand.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[38,0,"real","derived",true,true,true,"alternate",1]`, string(got))
}

// TestSearchParamsProtectedSurface retains the installed helper contract while
// leaving unsupported iteration and general URL functionality unavailable.
func TestSearchParamsProtectedSurface(t *testing.T) {
	source := `(() => {"use strict";
		const p = new URLSearchParams("x=1");
		const attempts = [
			() => {globalThis.URLSearchParams = null;},
			() => {URLSearchParams.prototype = {};},
			() => {URLSearchParams.prototype.append = () => {};},
			() => {Object.defineProperty(URLSearchParams.prototype, "get", {value: null});},
			() => {Object.defineProperty(URLSearchParams.prototype, "size", {get() {return 99;}});},
			() => {p.size = 99;}
		];
		const rejected = attempts.every(attempt => {
			try {attempt(); return false;} catch (error) {return error instanceof TypeError;}
		});
		const properties = [[globalThis, "URLSearchParams"], [URLSearchParams, "prototype"],
			...(["append", "set", "get", "getAll", "has", "delete", "sort", "toString"]
				.map(name => [URLSearchParams.prototype, name]))];
		const protectedData = properties.every(([object, name]) => {
			const d = Object.getOwnPropertyDescriptor(object, name);
			return !d.writable && !d.configurable && !d.enumerable;
		});
		const size = Object.getOwnPropertyDescriptor(URLSearchParams.prototype, "size");
		return [rejected, protectedData, typeof size.get === "function" && size.set === undefined && !size.configurable,
			p.size, p.toString(), [typeof URL, typeof p.entries, typeof p.keys, typeof p.values,
				typeof p.forEach, typeof p[Symbol.iterator]]];
	})()`

	// Read-only host properties resist reassignment and descriptor replacement.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[true,true,true,1,"x=1",["undefined","undefined","undefined","undefined","undefined","undefined"]]`, string(got))
}

// TestSearchParamsNativeResults builds independent native arrays without
// consulting replaced constructors, array methods, or inherited index setters.
func TestSearchParamsNativeResults(t *testing.T) {
	source := `(() => {
		let touched = 0;
		const p = new URLSearchParams("x=first&x=second");
		const isArray = Array.isArray;
		const fail = () => {touched++; throw "privateDetail";};
		Object.defineProperty(Array.prototype, "0", {set: fail, configurable: true});
		Array.prototype.push = fail;
		Array = fail;
		Object.getOwnPropertyDescriptor = fail;
		const first = p.getAll("x");
		first[0] = "changed";
		const second = p.getAll("x");
		return [isArray(first), isArray(second), first, second, p.get("x"), touched];
	})()`

	// Mutating one returned array cannot change the retained tuples or a later result.
	got, err := evaluateSource(context.Background(), searchParamsLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[true,true,["changed","second"],["first","second"],"first",0]`, string(got))
}
