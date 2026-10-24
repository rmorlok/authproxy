package httpcodec

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// responseStatusPointer supplies independently mutable optional scalar values
// for source definitions without coupling tests to production helper functions.
func responseStatusPointer[T any](value T) *T {
	return &value
}

// TestResponseRulesDefaults covers absent policies, required limits, and the
// explicit refusal to ignore an authored JavaScript transform.
func TestResponseRulesDefaults(t *testing.T) {
	for _, source := range []*tools.HTTPResponse{nil, {}} {
		// An absent or empty definition compiles without introducing any mapping.
		rules, err := compileResponseRules(source, 1)
		require.NoError(t, err)
		require.NotNil(t, rules)
		for _, status := range []int{100, 200, 404, 599} {
			rule, matched := rules.lookup(status)
			require.False(t, matched)
			require.Zero(t, rule)
		}
	}
	for _, limit := range []int{0, -1} {
		// Positive limits remain mandatory even when no source bytes are present.
		rules, err := compileResponseRules(nil, limit)
		require.ErrorContains(t, err, "positive")
		require.Nil(t, rules)
	}
	for _, expression := range []string{"", "response.body", "private-expression"} {
		// Every present transform fails instead of silently returning passthrough data.
		rules, err := compileResponseRules(&tools.HTTPResponse{TransformJavascript: &expression}, 1024)
		require.ErrorContains(t, err, "transforms are not supported")
		require.NotContains(t, err.Error(), "private-expression")
		require.Nil(t, rules)
	}
}

// TestResponseRulesPrecedence verifies that selector priority is independent of
// declaration order and that explicit selectors can map successful HTTP.
func TestResponseRulesPrecedence(t *testing.T) {
	source := []tools.HTTPErrorRule{
		{Default: true, Code: "FALLBACK", Message: "fallback"},
		{StatusClass: responseStatusPointer(4), Code: "CLIENT", Message: "client failure"},
		{Statuses: []int{404, 410}, Code: "MISSING", Message: "not found", Retryable: responseStatusPointer(true)},
		{StatusClass: responseStatusPointer(2), Code: "SUCCESS_CLASS", Message: "provider reports a failure"},
		{Statuses: []int{200}, Code: "SUCCESS_EXACT", Message: "provider reports another failure"},
	}
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			// Reversing declarations leaves exact/class/default priority unchanged.
			ordered := append([]tools.HTTPErrorRule(nil), source...)
			if reverse {
				for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
					ordered[left], ordered[right] = ordered[right], ordered[left]
				}
			}
			rules, err := compileResponseRules(&tools.HTTPResponse{Errors: ordered}, 1024)
			require.NoError(t, err)
			for _, expected := range []struct {
				status    int
				code      string
				retryable bool
			}{
				{100, "FALLBACK", false}, {200, "SUCCESS_EXACT", false},
				{204, "SUCCESS_CLASS", false}, {302, "FALLBACK", false},
				{400, "CLIENT", false}, {404, "MISSING", true},
				{410, "MISSING", true}, {503, "FALLBACK", false},
			} {
				rule, matched := rules.lookup(expected.status)
				require.True(t, matched)
				require.Equal(t, expected.code, rule.code)
				require.Equal(t, expected.retryable, rule.retryable)
			}
		})
	}

	// A default error rule preserves every 2xx response for successful mapping.
	rules, err := compileResponseRules(&tools.HTTPResponse{Errors: source[:1]}, 1024)
	require.NoError(t, err)
	for status := 200; status < 300; status++ {
		rule, matched := rules.lookup(status)
		require.False(t, matched, "default error rule must not match status %d", status)
		require.Zero(t, rule)
	}
	// Statuses immediately outside the success range still use the fallback.
	for _, status := range []int{100, 199, 300, 599} {
		rule, matched := rules.lookup(status)
		require.True(t, matched)
		require.Equal(t, "FALLBACK", rule.code)
	}
	// Invalid statuses never become authored errors, even with a default rule.
	for _, status := range []int{-1, 0, 99, 600} {
		rule, matched := rules.lookup(status)
		require.False(t, matched)
		require.Zero(t, rule)
	}
	var absent *responseRules
	_, matched := absent.lookup(404)
	require.False(t, matched)
}

// TestResponseRulesUnmatched preserves absence of an error mapping when neither
// an exact selector, a matching class, nor a default covers the status.
func TestResponseRulesUnmatched(t *testing.T) {
	// The class and exact selector can overlap, but do not imply a fallback.
	rules, err := compileResponseRules(&tools.HTTPResponse{Errors: []tools.HTTPErrorRule{
		{Statuses: []int{404}, Code: "MISSING", Message: "missing"},
		{StatusClass: responseStatusPointer(4), Code: "CLIENT", Message: "client failure"},
	}}, 1024)
	require.NoError(t, err)
	for _, status := range []int{200, 302, 500} {
		rule, matched := rules.lookup(status)
		require.False(t, matched)
		require.Zero(t, rule)
	}
}

// TestResponseRulesValidation applies the authored schema's selector and field
// constraints before creating a usable compiled policy.
func TestResponseRulesValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules []tools.HTTPErrorRule
	}{
		{"no selector", []tools.HTTPErrorRule{{Code: "CUSTOM", Message: "safe"}}},
		{"empty statuses", []tools.HTTPErrorRule{{Statuses: []int{}, Code: "CUSTOM", Message: "safe"}}},
		{"two selectors", []tools.HTTPErrorRule{{Statuses: []int{404}, Default: true, Code: "CUSTOM", Message: "safe"}}},
		{"duplicate status within rule", []tools.HTTPErrorRule{{Statuses: []int{404, 404}, Code: "CUSTOM", Message: "safe"}}},
		{"duplicate status across rules", []tools.HTTPErrorRule{{Statuses: []int{404}, Code: "FIRST", Message: "safe"}, {Statuses: []int{404}, Code: "SECOND", Message: "safe"}}},
		{"duplicate class", []tools.HTTPErrorRule{{StatusClass: responseStatusPointer(4), Code: "FIRST", Message: "safe"}, {StatusClass: responseStatusPointer(4), Code: "SECOND", Message: "safe"}}},
		{"duplicate default", []tools.HTTPErrorRule{{Default: true, Code: "FIRST", Message: "safe"}, {Default: true, Code: "SECOND", Message: "safe"}}},
		{"low status", []tools.HTTPErrorRule{{Statuses: []int{99}, Code: "CUSTOM", Message: "safe"}}},
		{"high status", []tools.HTTPErrorRule{{Statuses: []int{600}, Code: "CUSTOM", Message: "safe"}}},
		{"low class", []tools.HTTPErrorRule{{StatusClass: responseStatusPointer(0), Code: "CUSTOM", Message: "safe"}}},
		{"high class", []tools.HTTPErrorRule{{StatusClass: responseStatusPointer(6), Code: "CUSTOM", Message: "safe"}}},
		{"empty code", []tools.HTTPErrorRule{{Default: true, Message: "safe"}}},
		{"code surrounding whitespace", []tools.HTTPErrorRule{{Default: true, Code: " CUSTOM ", Message: "safe"}}},
		{"empty message", []tools.HTTPErrorRule{{Default: true, Code: "CUSTOM"}}},
		{"blank message", []tools.HTTPErrorRule{{Default: true, Code: "CUSTOM", Message: " \t "}}},
		{"invalid UTF-8 code", []tools.HTTPErrorRule{{Default: true, Code: "CUSTOM\xff", Message: "safe"}}},
		{"invalid UTF-8 message", []tools.HTTPErrorRule{{Default: true, Code: "CUSTOM", Message: "private-message\xff"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Invalid input returns no partial policy and does not expose message contents.
			rules, err := compileResponseRules(&tools.HTTPResponse{Errors: test.rules}, 1024)
			require.Error(t, err)
			require.Nil(t, rules)
			require.NotContains(t, err.Error(), "private-message")
		})
	}
}

// TestResponseRulesReservedCodes protects platform-defined failures while
// retaining custom identifiers and messages exactly as authored.
func TestResponseRulesReservedCodes(t *testing.T) {
	for _, code := range []string{
		"INPUT_INVALID", "CONNECTION_UNAVAILABLE", "UPSTREAM_AUTH_FAILED",
		"UPSTREAM_FAILED", "UPSTREAM_PROTOCOL_ERROR", "UNSUPPORTED_CAPABILITY",
		"EXECUTION_LIMIT_EXCEEDED", "OUTPUT_INVALID", "OUTPUT_TOO_LARGE", "TOOL_REVISION_CHANGED",
	} {
		t.Run(code, func(t *testing.T) {
			// A provider-specific rule may not claim to be a platform failure.
			rules, err := compileResponseRules(&tools.HTTPResponse{Errors: []tools.HTTPErrorRule{{Default: true, Code: code, Message: "safe"}}}, 1024)
			require.ErrorContains(t, err, "reserved platform codes")
			require.Nil(t, rules)
		})
	}
	for _, code := range []string{"RATE_LIMITED", "upstream_failed", "CUSTOM_é"} {
		// Custom code casing and Unicode are not constrained beyond the schema.
		rules, err := compileResponseRules(&tools.HTTPResponse{Errors: []tools.HTTPErrorRule{{Default: true, Code: code, Message: "  安全な説明  "}}}, 1024)
		require.NoError(t, err)
		rule, matched := rules.lookup(429)
		require.True(t, matched)
		require.Equal(t, code, rule.code)
		require.Equal(t, "  安全な説明  ", rule.message)
	}
}

// TestResponseRulesSourceByteLimits exercises aggregate source limits exactly
// at their boundary, including selector and explicit retryability values.
func TestResponseRulesSourceByteLimits(t *testing.T) {
	for _, test := range []struct {
		name string
		rule tools.HTTPErrorRule
		size int
	}{
		{"statuses", tools.HTTPErrorRule{Statuses: []int{400, 401}, Code: "CUSTOM", Message: "safe"}, 1 + 6 + 4 + 6},
		{"class", tools.HTTPErrorRule{StatusClass: responseStatusPointer(4), Code: "CUSTOM", Message: "safe"}, 1 + 6 + 4 + 1},
		{"default", tools.HTTPErrorRule{Default: true, Code: "CUSTOM", Message: "safe"}, 1 + 6 + 4 + 4},
		{"retryable true", tools.HTTPErrorRule{Default: true, Code: "CUSTOM", Message: "safe", Retryable: responseStatusPointer(true)}, 1 + 6 + 4 + 4 + 4},
		{"retryable false", tools.HTTPErrorRule{Default: true, Code: "CUSTOM", Message: "safe", Retryable: responseStatusPointer(false)}, 1 + 6 + 4 + 4 + 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The same definition fits at its limit and fails one byte below it.
			source := &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{test.rule}}
			rules, err := compileResponseRules(source, test.size)
			require.NoError(t, err)
			require.NotNil(t, rules)
			rules, err = compileResponseRules(source, test.size-1)
			require.ErrorContains(t, err, "byte limit")
			require.Nil(t, rules)
		})
	}

	// Rules share a single allowance; separately small strings cannot exceed it collectively.
	source := &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{
		{Statuses: []int{404}, Code: "C", Message: "M"},
		{Default: true, Code: "C", Message: "M"},
	}}
	_, err := compileResponseRules(source, 13)
	require.NoError(t, err)
	_, err = compileResponseRules(source, 12)
	require.ErrorContains(t, err, "byte limit")

	// Oversized strings are rejected before whitespace and UTF-8 validation scans.
	source.Errors[0].Message = strings.Repeat(" ", 1024)
	_, err = compileResponseRules(source, 100)
	require.ErrorContains(t, err, "byte limit")
}

// TestResponseRulesSelectorWorkLimits bounds malformed selector collections
// before schema validation and still accepts the complete valid selector space.
func TestResponseRulesSelectorWorkLimits(t *testing.T) {
	// A valid policy may cover every exact status, all five classes, and a default.
	source := &tools.HTTPResponse{}
	for status := 100; status <= 599; status++ {
		source.Errors = append(source.Errors, tools.HTTPErrorRule{Statuses: []int{status}, Code: "C", Message: "M"})
	}
	for class := 1; class <= 5; class++ {
		source.Errors = append(source.Errors, tools.HTTPErrorRule{StatusClass: responseStatusPointer(class), Code: "C", Message: "M"})
	}
	source.Errors = append(source.Errors, tools.HTTPErrorRule{Default: true, Code: "C", Message: "M"})
	rules, err := compileResponseRules(source, 4096)
	require.NoError(t, err)
	require.NotNil(t, rules)

	// A 507th rule cannot be valid, so it fails the work bound before selector validation.
	source.Errors = append(source.Errors, tools.HTTPErrorRule{})
	rules, err = compileResponseRules(source, 4096)
	require.ErrorContains(t, err, "rule count")
	require.Nil(t, rules)
	for _, counts := range [][]int{{501}, {250, 251}} {
		// Exact-status entries share a count cap across all rules, even if duplicates are invalid.
		source := &tools.HTTPResponse{}
		for _, count := range counts {
			source.Errors = append(source.Errors, tools.HTTPErrorRule{Statuses: make([]int, count)})
		}
		rules, err := compileResponseRules(source, 4096)
		require.ErrorContains(t, err, "exact status count")
		require.Nil(t, rules)
	}
}

// TestResponseRulesCaptureAndConcurrentLookup ensures compilation detaches all
// mutable source selectors and optional flags, and supports concurrent reuse.
func TestResponseRulesCaptureAndConcurrentLookup(t *testing.T) {
	source := &tools.HTTPResponse{Errors: []tools.HTTPErrorRule{
		{Statuses: []int{404}, Code: "MISSING", Message: "missing", Retryable: responseStatusPointer(true)},
		{StatusClass: responseStatusPointer(4), Code: "CLIENT", Message: "client"},
		{Default: true, Code: "FALLBACK", Message: "fallback", Retryable: responseStatusPointer(true)},
	}}
	rules, err := compileResponseRules(source, 1024)
	require.NoError(t, err)

	// Change every retained source category after compilation without changing the policy.
	source.Errors[0].Statuses[0] = 500
	*source.Errors[0].Retryable = false
	*source.Errors[1].StatusClass = 5
	*source.Errors[2].Retryable = false
	for index := range source.Errors {
		source.Errors[index].Code = "CHANGED"
		source.Errors[index].Message = "changed"
	}
	expected := map[int]responseRule{
		404: {code: "MISSING", message: "missing", retryable: true},
		400: {code: "CLIENT", message: "client"},
		500: {code: "FALLBACK", message: "fallback", retryable: true},
	}
	for status, want := range expected {
		rule, matched := rules.lookup(status)
		require.True(t, matched)
		require.Equal(t, want, rule)
		// A returned value is also detached from the internal map or fallback pointer.
		rule.code = "CHANGED AGAIN"
		again, _ := rules.lookup(status)
		require.Equal(t, want, again)
	}

	// Many readers can share the compiled policy without synchronization or mutation.
	const readers = 24
	var readersDone sync.WaitGroup
	failures := make(chan error, readers)
	for reader := 0; reader < readers; reader++ {
		readersDone.Add(1)
		go func() {
			defer readersDone.Done()
			for iteration := 0; iteration < 100; iteration++ {
				for status, want := range expected {
					if rule, matched := rules.lookup(status); !matched || rule != want {
						failures <- fmt.Errorf("unexpected mapping for status %d", status)
						return
					}
				}
			}
		}()
	}
	readersDone.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
}
