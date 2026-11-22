package runtime_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// base64TestLimits supplies room for ordinary shared-budget base64 fixtures.
func base64TestLimits() jsruntime.Limits { return runtimeLimits() }

// TestBase64ForgivingDecode accepts the standard alphabet, optional padding,
// the five ASCII whitespace characters, and unused nonzero trailing bits.
func TestBase64ForgivingDecode(t *testing.T) {
	for _, tt := range []struct{ encoded, decoded string }{
		{"", ""},
		{" \t\n\f\r", ""},
		{"Zg==", "f"},
		{"Zg", "f"},
		{"Zh==", "f"},
		{"Zh", "f"},
		{"Zm8=", "fo"},
		{"Zm8", "fo"},
		{"Zm9=", "fo"},
		{"Zm9", "fo"},
		{"Zm9v", "foo"},
		{"\tZ\nm\f9\rv ", "foo"},
		{" Z g = = ", "f"},
	} {
		t.Run(fmt.Sprintf("%q", tt.encoded), func(t *testing.T) {
			// JSON quoting embeds control characters without changing the authored argument.
			argument, err := json.Marshal(tt.encoded)
			require.NoError(t, err)
			got, err := evaluateSource(context.Background(), base64TestLimits(), "atob("+string(argument)+")")
			require.NoError(t, err)
			var decoded string
			require.NoError(t, json.Unmarshal(got, &decoded))
			require.Equal(t, tt.decoded, decoded)
		})
	}
}

// TestBase64BinaryFidelity verifies all 256 byte values rather than treating
// binary strings as UTF-8 or accidentally replacing bytes above ASCII.
func TestBase64BinaryFidelity(t *testing.T) {
	bytes, codes := make([]byte, 256), make([]int, 256)
	for i := range bytes {
		bytes[i], codes[i] = byte(i), i
	}
	encoded := base64.StdEncoding.EncodeToString(bytes)
	want, err := json.Marshal(map[string]any{"encoded": encoded, "codes": codes})
	require.NoError(t, err)

	// Compare encoding to an independent fixture and decode that fixture into code units.
	source := fmt.Sprintf(`({
		encoded: btoa(String.fromCharCode(...Array.from({length:256}, (_,i) => i))),
		codes: Array.from(atob(%q), character => character.charCodeAt(0))
	})`, encoded)
	got, err := evaluateSource(context.Background(), base64TestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))

	// Empty strings are valid; Latin-1 input uses one work unit beyond its helper entry.
	limits := base64TestLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 4, 4
	got, err = runUtilitySource(context.Background(), limits, `[btoa(""), atob(""), btoa("\u00ff")]`)
	require.NoError(t, err)
	require.JSONEq(t, `["","","/w=="]`, string(got))
}

// TestBase64MalformedInput produces catchable TypeErrors without exposing input
// or invoking authored conversion methods, even for boxed strings and proxies.
func TestBase64MalformedInput(t *testing.T) {
	for helper, arguments := range map[string][]string{
		"atob": {`"A"`, `"Zg="`, `"Zg==="`, `"=Zg="`, `"Zg==AA=="`, `"-w=="`, `"____"`, `"Zg\v=="`, `"\u00a0Zg=="`, `"é"`, `"\ud800"`, `"privateDetail!"`},
		"btoa": {`"\u0100"`, `"雪"`, `"😀"`, `"\ud800"`, `"\udfff"`, `"privateDetail\u0100"`},
	} {
		for _, argument := range append(arguments, ``, `undefined`, `null`, `42`, `true`,
			`new String("privateDetail")`, `{toString() {for (;;) {}}}`, `new Proxy({}, {get() {for (;;) {}}})`) {
			t.Run(helper+"("+argument+")", func(t *testing.T) {
				// A normal malformed call is catchable; private details never enter its message.
				source := fmt.Sprintf(`(() => {try {%s(%s); return "accepted";}
					catch (error) {return [error instanceof TypeError, error.message.includes("privateDetail")];}})()`, helper, argument)
				got, err := evaluateSource(context.Background(), base64TestLimits(), source)
				require.NoError(t, err)
				require.JSONEq(t, `[true,false]`, string(got))
			})
		}
	}
}

// TestBase64SharedExactBudgets charges both helpers to one invocation allowance
// and preserves a limit failure even if authored JavaScript catches its error.
func TestBase64SharedExactBudgets(t *testing.T) {
	limits := base64TestLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 5, 5
	source := `[btoa("f"),atob("Zg")]`

	// Entries and scanned units share work; produced values share allocation allowance.
	got, err := runUtilitySource(context.Background(), limits, source)
	require.NoError(t, err)
	require.JSONEq(t, `["Zg==","f"]`, string(got))
	for name, change := range map[string]func(*jsruntime.Limits){
		"work":       func(p *jsruntime.Limits) { p.MaxWorkUnits-- },
		"allocation": func(p *jsruntime.Limits) { p.MaxAllocationUnits-- },
	} {
		t.Run(name, func(t *testing.T) {
			tight := limits
			change(&tight)
			got, err := runUtilitySource(context.Background(), tight, `try {`+source+`;} catch (_) {} "privateDetail";`)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, got)
			require.NotContains(t, err.Error(), "privateDetail")
		})
	}
}

// TestBase64AttemptAccounting includes malformed inputs and whitespace in the
// relevant allowances without charging output that an invalid call never emits.
func TestBase64AttemptAccounting(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		work, output int
		want         string
	}{
		{"whitespace exact", `atob(" Zg== ")`, 7, 1, `"f"`},
		{"whitespace over", `atob(" Zg== ")`, 6, 1, ""},
		{"decoded binary exact", `atob("////").split("").map(c => c.charCodeAt(0))`, 5, 3, `[255,255,255]`},
		{"decoded binary over", `atob("////")`, 5, 2, ""},
		{"empty attempt", `btoa(""); atob("");`, 1, 1, ""},
		{"malformed consumes work", `try {atob("!");} catch (_) {} btoa("a");`, 3, 4, ""},
		{"malformed emits nothing", `try {atob("!");} catch (_) {} btoa("a");`, 4, 4, `"YQ=="`},
		{"malformed Unicode units", `try {btoa("😀");} catch (_) {} btoa("a");`, 5, 4, `"YQ=="`},
		{"Unicode input exceeded", `try {btoa("😀");} catch (_) {} btoa("a");`, 4, 4, ""},
		{"non-string consumes entry work", `try {btoa(null);} catch (_) {} btoa("a");`, 2, 4, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			limits := base64TestLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = tt.work, tt.output
			// Calls within budget succeed; any exhausted allowance invalidates the entire result.
			got, err := runUtilitySource(context.Background(), limits, tt.source)
			if tt.want == "" {
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, tt.want, string(got))
			}
		})
	}
}

// TestBase64Cancellation stops repeated helper work under the common runtime
// deadline while distinguishing caller cancellation from policy expiration.
func TestBase64Cancellation(t *testing.T) {
	limits := base64TestLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 1<<30, 1<<30
	source := `for (;;) {atob(btoa("abc"));}`

	// Generous execution allowances let the caller's earlier deadline stop the loop.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	got, err := evaluateSource(ctx, limits, source)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, got)

	// Without a caller deadline, the same work ends with the runtime's limit category.
	limits.Timeout = 25 * time.Millisecond
	got, err = evaluateSource(context.Background(), limits, source)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, got)
}

// base64PrimitiveString avoids an embedded field named String shadowing the
// String method required by goja.Value.
type base64PrimitiveString = goja.String

// base64CancelString cancels a real context partway through a native scan.
// Embedding a primitive string retains its normal behavior while CharAt makes
// the cancellation point deterministic without timers or another goroutine.
type base64CancelString struct {
	base64PrimitiveString
	cancel      context.CancelFunc // Cancels the caller and its runtime context.
	cancelAfter int                // Code-unit reads before cancellation begins.
	reads       int                // Reads completed on the VM owner goroutine.
}

// CharAt cancels at the selected read and then preserves the underlying unit.
func (s *base64CancelString) CharAt(index int) uint16 {
	s.reads++
	if s.reads == s.cancelAfter {
		s.cancel()
	}
	return s.base64PrimitiveString.CharAt(index)
}

// TestBase64CancellationDuringNativeWork stops inside either native pass on a
// large input, without depending on a polling interval or wall-clock timing.
func TestBase64CancellationDuringNativeWork(t *testing.T) {
	text := strings.Repeat("A", 64<<10)
	for _, helper := range []string{"atob", "btoa"} {
		for _, phase := range []struct {
			name          string
			cancelAfter   int
			fullPassReads int
		}{
			{"validation", 128, len(text)},
			{"conversion", len(text) + 128, 2 * len(text)},
		} {
			t.Run(helper+"/"+phase.name, func(t *testing.T) {
				// Both helpers accept this input and have enough budget for a full result.
				limits := base64TestLimits()
				limits.Timeout = 10 * time.Second
				limits.MaxInputBytes = len(text)
				limits.MaxWorkUnits = len(text) + 1
				limits.MaxAllocationUnits = 2 * len(text)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				reads := 0
				got, err := jsruntime.EvaluateJSON(ctx, limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
					// Keep the wrapped primitive and helper entirely inside their owning VM.
					input := &base64CancelString{
						base64PrimitiveString: vm.ToValue(text).(goja.String), cancel: cancel, cancelAfter: phase.cancelAfter,
					}
					defer func() { reads = input.reads }()
					call, ok := goja.AssertFunction(vm.Get(helper))
					require.True(t, ok)
					value, err := call(goja.Undefined(), input)
					return value, jsruntime.EvaluationError(err)
				})

				// Cancellation occurs during work and prevents the active pass from finishing.
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, got)
				require.GreaterOrEqual(t, reads, phase.cancelAfter)
				require.Less(t, reads, phase.fullPassReads)
			})
		}
	}
}

// BenchmarkBase64LargeInput measures complete evaluations of one-mebibyte
// helper inputs, returning only their lengths so output JSON stays small.
func BenchmarkBase64LargeInput(b *testing.B) {
	for _, test := range []struct {
		helper string
		input  string
		length int
	}{
		{"btoa", strings.Repeat("a", 1<<20), base64.StdEncoding.EncodedLen(1 << 20)},
		{"atob", strings.Repeat("YWFh", 1<<18), 3 << 18},
	} {
		b.Run(test.helper, func(b *testing.B) {
			// Compile once; every evaluation still gets its own VM and finite allowances.
			limits := base64TestLimits()
			limits.Timeout = 10 * time.Second
			limits.MaxInputBytes = len(test.input)
			limits.MaxWorkUnits = 2*len(test.input) + 32
			limits.MaxAllocationUnits = 2*len(test.input) + 64
			program, err := jsruntime.CompileExpression(test.helper+"(data).length", limits)
			require.NoError(b, err)
			want := fmt.Sprint(test.length)
			b.ReportAllocs()
			b.SetBytes(int64(len(test.input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// The trusted preparation callback supplies the known, bounded native fixture.
				got, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime) error {
					return vm.Set("data", test.input)
				})
				if err != nil {
					b.Fatal(err)
				}
				if string(got) != want {
					b.Fatalf("unexpected result length: %s", got)
				}
			}
		})
	}
}
