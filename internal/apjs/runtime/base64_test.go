package runtime_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// base64TestLimits enables both pure helpers with explicit shared allowances.
func base64TestLimits() jsruntime.Limits {
	limits := runtimeLimits()
	limits.Utilities = jsruntime.UtilityPolicy{
		Base64: true, MaxCalls: 16, MaxInputBytes: 4096, MaxOutputBytes: 4096,
	}
	return limits
}

// TestBase64ForgivingDecode accepts the standard alphabet, optional padding,
// the five ASCII whitespace characters, and unused nonzero trailing bits.
func TestBase64ForgivingDecode(t *testing.T) {
	for _, tt := range []struct{ encoded, decoded string }{
		{"", ""}, {" \t\n\f\r", ""}, {"Zg==", "f"}, {"Zg", "f"},
		{"Zh==", "f"}, {"Zh", "f"}, {"Zm8=", "fo"}, {"Zm8", "fo"},
		{"Zm9=", "fo"}, {"Zm9", "fo"}, {"Zm9v", "foo"},
		{"\tZ\nm\f9\rv ", "foo"}, {" Z g = = ", "f"},
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

	// Empty strings are valid, and a Latin-1 code unit occupies one utility input byte.
	limits := base64TestLimits()
	limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = 1, 4
	got, err = evaluateSource(context.Background(), limits, `[btoa(""), atob(""), btoa("\u00ff")]`)
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
	limits.Utilities.MaxCalls = 2
	limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = 3, 5
	source := `[btoa("f"),atob("Zg")]`

	// One encode and one decode exactly consume all three aggregate allowances.
	got, err := evaluateSource(context.Background(), limits, source)
	require.NoError(t, err)
	require.JSONEq(t, `["Zg==","f"]`, string(got))
	for name, change := range map[string]func(*jsruntime.UtilityPolicy){
		"calls":  func(p *jsruntime.UtilityPolicy) { p.MaxCalls-- },
		"input":  func(p *jsruntime.UtilityPolicy) { p.MaxInputBytes-- },
		"output": func(p *jsruntime.UtilityPolicy) { p.MaxOutputBytes-- },
	} {
		t.Run(name, func(t *testing.T) {
			tight := limits
			change(&tight.Utilities)
			got, err := evaluateSource(context.Background(), tight, `try {`+source+`;} catch (_) {} "privateDetail";`)
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
		name, source         string
		calls, input, output int
		want                 string
	}{
		{"whitespace exact", `atob(" Zg== ")`, 1, 6, 1, `"f"`},
		{"whitespace over", `atob(" Zg== ")`, 1, 5, 1, ""},
		{"decoded binary exact", `atob("////").split("").map(c => c.charCodeAt(0))`, 1, 4, 3, `[255,255,255]`},
		{"decoded binary over", `atob("////")`, 1, 4, 2, ""},
		{"empty attempt", `btoa(""); atob("");`, 1, 1, 1, ""},
		{"malformed consumes calls", `try {atob("!");} catch (_) {} btoa("a");`, 1, 2, 4, ""},
		{"malformed consumes input", `try {atob("!");} catch (_) {} btoa("a");`, 2, 1, 4, ""},
		{"malformed emits nothing", `try {atob("!");} catch (_) {} btoa("a");`, 2, 2, 4, `"YQ=="`},
		{"malformed Unicode units", `try {btoa("😀");} catch (_) {} btoa("a");`, 2, 3, 4, `"YQ=="`},
		{"Unicode input exceeded", `try {btoa("😀");} catch (_) {} btoa("a");`, 2, 2, 4, ""},
		{"non-string consumes calls", `try {btoa(null);} catch (_) {} btoa("a");`, 1, 1, 4, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			limits := base64TestLimits()
			limits.Utilities.MaxCalls = tt.calls
			limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = tt.input, tt.output
			// Calls within budget succeed; any exhausted allowance invalidates the entire result.
			got, err := evaluateSource(context.Background(), limits, tt.source)
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
	limits.Utilities.MaxCalls = 1 << 30
	limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = 1<<30, 1<<30
	source := `for (;;) {atob(btoa("abc"));}`

	// Generous utility counters let the caller's earlier deadline stop the loop.
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
