package runtime

import (
	"context"
	"strconv"
	"testing"

	"github.com/dop251/goja"
	whatwgurl "github.com/nlnwa/whatwg-url/url"
	"github.com/stretchr/testify/require"
)

// TestURLNativeAdmissionOverflow rejects a quadratic cost before multiplication
// can wrap into a small positive value or zero and bypass the shared limits.
func TestURLNativeAdmissionOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, size := range []int{maxInt, 1 << (strconv.IntSize / 2)} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			// Maximum logical caps admit real costs but must not permit overflow.
			ctx, err := WithExecutionBudget(context.Background(), Limits{
				MaxWorkUnits: maxInt, MaxAllocationUnits: maxInt,
			})
			require.NoError(t, err)
			host := newURLHost(&utilityHost{ctx: ctx, vm: goja.New()})
			host.admitNative(7)
			budget := executionBudgetFromContext(ctx)
			require.Equal(t, 49, budget.workUnits)
			require.Equal(t, 49, budget.allocationUnits)

			// Overflow poisons the execution without adding either wrapped charge.
			exception := host.host.vm.Try(func() { host.admitNative(size) })
			require.NotNil(t, exception)
			require.ErrorIs(t, budget.err(), ErrLimitExceeded)
			require.Equal(t, 49, budget.workUnits)
			require.Equal(t, 49, budget.allocationUnits)
		})
	}
}

// cancelingURLParser cancels immediately after the real parser finishes the
// selected stage, without timing races or changes to production parsing code.
type cancelingURLParser struct {
	whatwgurl.Parser
	cancel          context.CancelFunc // Cancels the host before its next context check.
	cancelAfterBase bool               // Selects Parse rather than final BasicParser.
	parseCalls      int                // Counts base parses reached before cancellation.
	basicCalls      int                // Counts final parses reached before cancellation.
	nativeErr       error              // Records the selected parser's actual outcome.
}

// Parse cancels after base parsing when that is the selected native stage.
func (p *cancelingURLParser) Parse(input string) (*whatwgurl.Url, error) {
	p.parseCalls++
	parsed, err := p.Parser.Parse(input)
	if p.cancelAfterBase {
		p.nativeErr = err
		p.cancel()
	}
	return parsed, err
}

// BasicParser cancels after final parsing when that is the selected native stage.
func (p *cancelingURLParser) BasicParser(input string, base, target *whatwgurl.Url, state whatwgurl.State) (*whatwgurl.Url, error) {
	p.basicCalls++
	parsed, err := p.Parser.BasicParser(input, base, target, state)
	if !p.cancelAfterBase {
		p.nativeErr = err
		p.cancel()
	}
	return parsed, err
}

// TestURLParserReturnCancellation stops before publishing a parsed receiver and
// gives cancellation precedence over either success or private parser errors.
func TestURLParserReturnCancellation(t *testing.T) {
	for _, test := range []struct {
		name, input, base string
		cancelAfterBase   bool
		malformed         bool
	}{
		{"valid base", "/item", "https://example.test/", true, false},
		{"malformed base", "/item", "https://privateDetail.test:wrong/", true, true},
		{"valid input", "/item", "https://example.test/", false, false},
		{"malformed input", "https://privateDetail.test:wrong/", "https://example.test/", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Wrap the real parser so cancellation happens after admitted native work.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			budgetCtx, err := WithExecutionBudget(ctx, Limits{
				MaxWorkUnits: 1 << 20, MaxAllocationUnits: 1 << 20,
			})
			require.NoError(t, err)
			vm := goja.New()
			host := newURLHost(&utilityHost{ctx: budgetCtx, vm: vm})
			parser := &cancelingURLParser{
				Parser: host.parser, cancel: cancel, cancelAfterBase: test.cancelAfterBase,
			}
			host.parser = parser
			receiver := vm.NewObject()
			var result goja.Value
			exception := vm.Try(func() {
				result = host.initialize(goja.FunctionCall{Arguments: []goja.Value{
					receiver, vm.ToValue(test.input), vm.ToValue(test.base),
				}})
			})

			// Cancellation wins before diagnostics or partial state become observable.
			require.NotNil(t, exception)
			require.Equal(t, "JavaScript utility execution canceled", exception.Value().ToObject(vm).Get("message").String())
			require.Nil(t, result)
			require.Empty(t, host.instances)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.NoError(t, executionBudgetFromContext(budgetCtx).err())
			require.Equal(t, 1, parser.parseCalls)
			if test.cancelAfterBase {
				require.Zero(t, parser.basicCalls)
			} else {
				require.Equal(t, 1, parser.basicCalls)
			}
			if test.malformed {
				require.Error(t, parser.nativeErr)
			} else {
				require.NoError(t, parser.nativeErr)
			}
		})
	}
}
