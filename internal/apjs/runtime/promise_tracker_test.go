package runtime

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestPromiseTrackerSettledResults checks root state and rejection accounting
// after Goja has drained its JavaScript microtasks.
func TestPromiseTrackerSettledResults(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   int64
		err    error
	}{
		"fulfilled":          {source: `Promise.resolve(4)`, want: 4},
		"microtasks":         {source: `(async () => (await Promise.resolve(4)) + 3)()`, want: 7},
		"root rejected":      {source: `Promise.reject("private reason")`, err: ErrEvaluation},
		"root stuck":         {source: `new Promise(() => {})`, err: ErrEvaluation},
		"detached rejection": {source: `Promise.reject("private reason"); Promise.resolve(4)`, err: ErrEvaluation},
		"handled rejection":  {source: `Promise.reject("private reason").catch(() => {}); Promise.resolve(4)`, want: 4},
		"partially handled":  {source: `Promise.reject("private").catch(() => {}); Promise.reject("another"); Promise.resolve(4)`, err: ErrEvaluation},
		"multiple handlers":  {source: `const rejected = Promise.reject("private"); rejected.catch(() => {}); rejected.catch(() => {}); Promise.resolve(4)`, want: 4},
	} {
		t.Run(name, func(t *testing.T) {
			// Install tracking before authored code creates or rejects any promises.
			vm := goja.New()
			tracker := NewPromiseTracker(vm)
			defer tracker.Close()
			value, err := vm.RunString(test.source)
			require.NoError(t, err)
			root, ok := value.Export().(*goja.Promise)
			require.True(t, ok)

			// Only a fulfilled root with no outstanding unhandled failure succeeds.
			value, err = tracker.SettledResult(root)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
				require.Nil(t, value)
				require.NotContains(t, err.Error(), "private")
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, value.ToInteger())
			}
		})
	}
}
