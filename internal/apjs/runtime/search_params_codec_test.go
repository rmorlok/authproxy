package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestSearchParamParsing preserves ordered duplicates and follows forgiving
// form-query decoding instead of Go's stricter net/url query parser.
func TestSearchParamParsing(t *testing.T) {
	for _, test := range []struct {
		name, input, serialized string
		pairs                   [][2]string
	}{
		{"empty", "", "", nil},
		{"question mark", "?", "", nil},
		{"empty segments", "?&&&&", "", nil},
		{"duplicate names", "?a=1&a=2&b", "a=1&a=2&b=", [][2]string{{"a", "1"}, {"a", "2"}, {"b", ""}}},
		{"empty tuples", "&=&=x&&y=&", "=&=x&y=", [][2]string{{"", ""}, {"", "x"}, {"y", ""}}},
		{"first equals", "x=a=b=c", "x=a%3Db%3Dc", [][2]string{{"x", "a=b=c"}}},
		{"only first question mark", "??a=1", "%3Fa=1", [][2]string{{"?a", "1"}}},
		{"plus versus encoded plus", "a+b=c+d%2be%2Bf", "a+b=c+d%2Be%2Bf", [][2]string{{"a b", "c d+e+f"}}},
		{"escaped delimiters", "a%26b=c%3dd%26e%3Ff%23g", "a%26b=c%3Dd%26e%3Ff%23g", [][2]string{{"a&b", "c=d&e?f#g"}}},
		{"malformed escapes", "%=%2&%q1=%Aq&x=%2g%20", "%25=%252&%25q1=%25Aq&x=%252g+", [][2]string{{"%", "%2"}, {"%q1", "%Aq"}, {"x", "%2g "}}},
		{"literal Unicode", "é=雪😀", "%C3%A9=%E9%9B%AA%F0%9F%98%80", [][2]string{{"é", "雪😀"}}},
		{"encoded Unicode", "%c3%a9=%e9%9b%aa%f0%9f%98%80", "%C3%A9=%E9%9B%AA%F0%9F%98%80", [][2]string{{"é", "雪😀"}}},
		{"BOM preserved", "%EF%BB%BF=%EF%BB%BFA", "%EF%BB%BF=%EF%BB%BFA", [][2]string{{"\ufeff", "\ufeffA"}}},
		{"NUL preserved", "%00=%00", "%00=%00", [][2]string{{"\x00", "\x00"}}},
		{"truncated UTF8", "x=%e2%82", "x=%EF%BF%BD", [][2]string{{"x", "\ufffd"}}},
		{"invalid continuation", "x=%E2%82A", "x=%EF%BF%BDA", [][2]string{{"x", "\ufffdA"}}},
		{"encoded surrogate", "x=%ED%A0%80", "x=%EF%BF%BD%EF%BF%BD%EF%BF%BD", [][2]string{{"x", "\ufffd\ufffd\ufffd"}}},
		{"full URL is query data", "https://example.test/?a=1#f", "https%3A%2F%2Fexample.test%2F%3Fa=1%23f", [][2]string{{"https://example.test/?a", "1#f"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Parsed strings are normalized data, with their original tuple order intact.
			host := textCodecTestHost(4096)
			pairs := host.parseSearchParams(host.vm.ToValue(test.input).(goja.String))
			require.Len(t, pairs, len(test.pairs))
			for index, pair := range pairs {
				require.Equal(t, test.pairs[index][0], pair.name.String())
				require.Equal(t, test.pairs[index][1], pair.value.String())
			}

			// Serialization canonicalizes escapes without sorting or losing duplicates.
			require.Equal(t, test.serialized, host.serializeSearchParams(pairs).String())
		})
	}
}

// TestSearchParamASCIIEncoding verifies the exact form-encoding allowlist over
// every ASCII byte, including controls and punctuation commonly left unescaped
// by other encoders such as encodeURIComponent.
func TestSearchParamASCIIEncoding(t *testing.T) {
	host := textCodecTestHost(4096)
	units := make([]uint16, 128)
	for index := range units {
		units[index] = uint16(index)
	}
	pairs := []searchParam{{name: host.vm.ToValue("").(goja.String), value: goja.StringFromUTF16(units)}}

	// This independent fixture distinguishes space, plus, star, and tilde rules.
	got := host.serializeSearchParams(pairs).String()
	want := "=%00%01%02%03%04%05%06%07%08%09%0A%0B%0C%0D%0E%0F" +
		"%10%11%12%13%14%15%16%17%18%19%1A%1B%1C%1D%1E%1F" +
		"+%21%22%23%24%25%26%27%28%29*%2B%2C-.%2F0123456789%3A%3B%3C%3D%3E%3F" +
		"%40ABCDEFGHIJKLMNOPQRSTUVWXYZ%5B%5C%5D%5E_%60abcdefghijklmnopqrstuvwxyz%7B%7C%7D%7E%7F"
	require.Equal(t, want, got)
	require.Equal(t, len(want), host.outputBytes)
}

// TestSearchParamScalarNormalization normalizes UTF-16 before query parsing or
// direct mutation, preserving pairs and replacing each isolated surrogate.
func TestSearchParamScalarNormalization(t *testing.T) {
	host := textCodecTestHost(128)
	input := goja.StringFromUTF16([]uint16{0xd800, '=', 0xdc00, 0xd83d, 0xde00})

	// Parse normalization happens before percent decoding and field splitting.
	pairs := host.parseSearchParams(input)
	require.Len(t, pairs, 1)
	require.Equal(t, []uint16{0xfffd}, textCodecUnits(pairs[0].name))
	require.Equal(t, []uint16{0xfffd, 0xd83d, 0xde00}, textCodecUnits(pairs[0].value))

	// The same conversion supports direct append/set values without interpreting delimiters.
	normalized := host.searchParamString(input)
	require.Equal(t, []uint16{0xfffd, '=', 0xfffd, 0xd83d, 0xde00}, textCodecUnits(normalized))
}

// TestSearchParamCodecAccounting includes temporary UTF-8, retained UTF-16,
// tuple slots, and each final serialized result in the shared output allowance.
func TestSearchParamCodecAccounting(t *testing.T) {
	const input = "?x=%E2%82&=&y=+"
	const serialized = "x=%EF%BF%BD&=&y=+"
	// The three tuples retain four text units (x, replacement, y, space).
	const parsedUnits = len(input) + 3 + 4
	host := textCodecTestHost(parsedUnits + len(serialized))

	pairs := host.parseSearchParams(host.vm.ToValue(input).(goja.String))
	require.Equal(t, parsedUnits, host.outputBytes)
	require.Equal(t, serialized, host.serializeSearchParams(pairs).String())
	require.Equal(t, parsedUnits+len(serialized), host.outputBytes)

	// A one-unit short result cannot allocate or return even a partial string.
	host = textCodecTestHost(parsedUnits + len(serialized) - 1)
	pairs = host.parseSearchParams(host.vm.ToValue(input).(goja.String))
	var result goja.Value
	exception := host.vm.Try(func() { result = host.serializeSearchParams(pairs) })
	require.NotNil(t, exception)
	require.Nil(t, result)
	require.ErrorIs(t, host.err, ErrLimitExceeded)
	require.Equal(t, parsedUnits, host.outputBytes)
}

// searchParamCancelContext cancels a real context at a chosen polling boundary
// so native-only parsing tests do not depend on wall-clock scheduling.
type searchParamCancelContext struct {
	context.Context
	cancel      context.CancelFunc // Cancels the embedded context once the selected check is reached.
	checks      int                // Observed native cancellation checks on the owning goroutine.
	cancelAfter int                // Check at which further native work must unwind.
}

// Err triggers cancellation deterministically, then returns the real context error.
func (c *searchParamCancelContext) Err() error {
	c.checks++
	if c.checks == c.cancelAfter {
		c.cancel()
	}
	return c.Context.Err()
}

// TestSearchParamCodecCancellation proves long native percent-decoding and
// serialization scans observe cancellation before producing their result.
func TestSearchParamCodecCancellation(t *testing.T) {
	for _, serialize := range []bool{false, true} {
		t.Run(map[bool]string{false: "percent decode", true: "serialize"}[serialize], func(t *testing.T) {
			host := textCodecTestHost(1 << 20)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			polling := &searchParamCancelContext{Context: ctx, cancel: cancel, cancelAfter: 2}
			host.ctx = polling
			var result goja.Value

			// Multiple polling intervals ensure cancellation occurs inside native work.
			exception := host.vm.Try(func() {
				if serialize {
					pairs := []searchParam{{
						name:  host.vm.ToValue(strings.Repeat("雪", 20000)).(goja.String),
						value: host.vm.ToValue("").(goja.String),
					}}
					result = host.serializeSearchParams(pairs)
				} else {
					result = host.decodeSearchParam([]byte(strings.Repeat("%41", 20000)))
				}
			})
			require.NotNil(t, exception)
			require.Nil(t, result)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, 2, polling.checks)
			require.Zero(t, host.outputBytes)
		})
	}
}

// TestSearchParamComparisonCancellation stops long shared prefixes inside the
// native UTF-16 comparator, before a caller can publish a comparison result.
func TestSearchParamComparisonCancellation(t *testing.T) {
	host := textCodecTestHost(1 << 20)
	params := newSearchParams(host)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	polling := &searchParamCancelContext{Context: ctx, cancel: cancel, cancelAfter: 2}
	host.ctx = polling
	first := host.vm.ToValue(strings.Repeat("a", 20000) + "b").(goja.String)
	second := host.vm.ToValue(strings.Repeat("a", 20000) + "c").(goja.String)
	result := 42

	// The second check occurs after one polling interval of matching code units.
	exception := host.vm.Try(func() { result = params.compare(first, second) })
	require.NotNil(t, exception)
	require.Equal(t, 42, result)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Equal(t, 2, polling.checks)
	require.Nil(t, host.err)
}

// TestSearchParamSortTailCancellation reaches a merge's comparison-free tail.
// Empty names make every comparison equal, so each merge first consumes its
// left half and then must poll while moving the remaining right-half tuples.
func TestSearchParamSortTailCancellation(t *testing.T) {
	const pairCount = 2 * textCodecPollInterval
	const previousPasses = 12 // Widths 1 through 2048 precede the final width-4096 merge.
	host := textCodecTestHost(1 << 20)
	host.policy.MaxInputBytes = 1 << 20
	params := newSearchParams(host)
	receiver := host.vm.NewObject()
	empty := host.vm.ToValue("").(goja.String)
	state := &searchParamsState{pairs: make([]searchParam, pairCount), scanUnits: pairCount}
	for index := range state.pairs {
		state.pairs[index] = searchParam{name: empty, value: empty}
	}
	params.instances[receiver] = state
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Each completed pass checks twice for tuple moves and once for each of
	// pairCount/2 empty-name comparisons. The final check is the tail boundary.
	polling := &searchParamCancelContext{
		Context: ctx, cancel: cancel,
		cancelAfter: 2 + (previousPasses+1)*(2+pairCount/2),
	}
	host.ctx = polling
	var result goja.Value

	exception := host.vm.Try(func() { result = params.sort(goja.FunctionCall{This: receiver}) })
	require.NotNil(t, exception)
	require.Nil(t, result)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Equal(t, polling.cancelAfter, polling.checks)

	// Scratch slots were admitted once. Cancellation stops before the first
	// final-tail move, after the earlier full passes and final left-half moves.
	require.Equal(t, pairCount, host.outputBytes)
	require.Equal(t, pairCount+previousPasses*pairCount+pairCount/2, host.inputBytes)
	require.Equal(t, 1, host.calls)
	require.Nil(t, host.err)
}

// TestSearchParamMutationReleasesRemovedReferences checks the backing storage,
// not merely the shortened slice, so deleted private values cannot be retained
// invisibly in spare tuple slots for the lifetime of their branded object.
func TestSearchParamMutationReleasesRemovedReferences(t *testing.T) {
	for _, test := range []struct {
		name, query, serialized string
		set                     bool
		arguments               []string
		retained                int
	}{
		{"set duplicates", "a=private-first&b=keep&a=private-second&a=private-third", "a=new&b=keep", true, []string{"a", "new"}, 2},
		{"delete selected values", "a=private&a=keep&b=keep&a=private", "a=keep&b=keep", false, []string{"a", "private"}, 2},
		{"delete all", "a=private-first&a=private-second", "", false, []string{"a"}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := textCodecTestHost(4096)
			params := newSearchParams(host)
			receiver := host.vm.NewObject()
			state := &searchParamsState{pairs: host.parseSearchParams(host.vm.ToValue(test.query).(goja.String))}
			params.recount(state)
			params.instances[receiver] = state
			backing := state.pairs
			arguments := make([]goja.Value, len(test.arguments))
			for index, value := range test.arguments {
				arguments[index] = host.vm.ToValue(value)
			}

			// Mutations retain the same backing slice while compacting survivors.
			call := goja.FunctionCall{This: receiver, Arguments: arguments}
			if test.set {
				params.set(call)
			} else {
				params.delete(call)
			}
			require.Len(t, state.pairs, test.retained)
			require.Equal(t, test.serialized, host.serializeSearchParams(state.pairs).String())

			// Spare capacity contains zeroed interfaces, including removed names.
			for _, pair := range backing[test.retained:] {
				require.Nil(t, pair.name)
				require.Nil(t, pair.value)
			}
		})
	}
}

// TestSearchParamTraversalCostOverflow rejects a cache update before wrapping
// its integer cost and accidentally admitting an unlimited later traversal.
func TestSearchParamTraversalCostOverflow(t *testing.T) {
	host := textCodecTestHost(16)
	params := newSearchParams(host)
	maximum := int(^uint(0) >> 1)
	state := &searchParamsState{scanUnits: maximum - 1}
	empty := host.vm.ToValue("").(goja.String)
	pair := searchParam{name: empty, value: empty}

	// The final representable empty tuple is safe; the next cannot wrap the cache.
	params.addUnits(state, pair)
	require.Equal(t, maximum, state.scanUnits)
	exception := host.vm.Try(func() { params.addUnits(state, pair) })
	require.NotNil(t, exception)
	require.ErrorIs(t, host.err, ErrLimitExceeded)
	require.Equal(t, maximum, state.scanUnits)
}
