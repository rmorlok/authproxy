package runtime

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestPromiseTrackerRejections checks counting after Goja drains its microtasks,
// without retaining the identities or reasons of ordinary rejected promises.
func TestPromiseTrackerRejections(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		count  int
	}{
		"fulfilled":          {source: `Promise.resolve(4)`},
		"microtasks":         {source: `(async () => (await Promise.resolve(4)) + 3)()`},
		"rejected":           {source: `Promise.reject("private reason")`, count: 1},
		"pending":            {source: `new Promise(() => {})`},
		"detached rejection": {source: `Promise.reject("private reason"); Promise.resolve(4)`, count: 1},
		"handled rejection":  {source: `Promise.reject("private reason").catch(() => {}); Promise.resolve(4)`},
		"partially handled":  {source: `Promise.reject("private").catch(() => {}); Promise.reject("another"); Promise.resolve(4)`, count: 1},
		"multiple handlers":  {source: `const rejected = Promise.reject("private"); rejected.catch(() => {}); rejected.catch(() => {}); Promise.resolve(4)`},
	} {
		t.Run(name, func(t *testing.T) {
			// Install the hook before authored code produces rejection notifications.
			vm := goja.New()
			tracker := NewPromiseTracker(vm)
			defer tracker.Close()
			_, err := vm.RunString(test.source)
			require.NoError(t, err)

			// Counting alone detects abandoned failures outside root capture.
			require.Equal(t, test.count, tracker.unhandled)
			require.Nil(t, tracker.initialUnhandled)
			if test.count == 0 {
				require.NoError(t, tracker.CheckRejections())
			} else {
				require.Equal(t, ErrEvaluation, tracker.CheckRejections())
			}
		})
	}
}

// trackerEntry compiles a callable without invoking it or creating its promises.
func trackerEntry(t *testing.T, vm *goja.Runtime, source string) goja.Callable {
	t.Helper()
	value, err := vm.RunString(source)
	require.NoError(t, err)
	callable, ok := goja.AssertFunction(value)
	require.True(t, ok)
	return callable
}

// TestPromiseTrackerCapturesSynchronousRoot accounts for notifications delivered
// before callAsync can obtain the root, including a root that is already handled.
func TestPromiseTrackerCapturesSynchronousRoot(t *testing.T) {
	for name, test := range map[string]struct {
		source        string
		count         int
		rootUnhandled bool
		err           error
	}{
		"fulfilled": {source: `() => Promise.resolve(4)`},
		"rejected": {
			source: `() => Promise.reject("private root")`, count: 1, rootUnhandled: true,
		},
		"handled root": {source: `() => {
			const root = Promise.reject("private root"); root.catch(() => {}); return root;
		}`},
		"handled root with unrelated rejection": {source: `() => {
			const root = Promise.reject("private root"); root.catch(() => {});
			Promise.reject("private detached"); return root;
		}`, count: 1, err: ErrEvaluation},
		"unhandled root with unrelated rejection": {source: `() => {
			Promise.reject("private detached"); return Promise.reject("private root");
		}`, count: 2, rootUnhandled: true, err: ErrEvaluation},
		"fulfilled root with unrelated rejection": {source: `() => {
			Promise.reject("private detached"); return Promise.resolve(4);
		}`, count: 1, err: ErrEvaluation},
	} {
		t.Run(name, func(t *testing.T) {
			vm := goja.New()
			tracker := NewPromiseTracker(vm)
			defer tracker.Close()
			entry := trackerEntry(t, vm, test.source)

			// Initial rejection events occur inside the callable, before its root is returned.
			root, err := tracker.callAsync(entry, goja.Undefined(), goja.Undefined())
			require.NoError(t, err)
			require.Same(t, root, tracker.root)
			require.Equal(t, test.count, tracker.unhandled)
			require.Equal(t, test.rootUnhandled, tracker.rootUnhandled)

			// Only the root remains referenced; an already-handled root exempts no failure.
			require.Nil(t, tracker.initialUnhandled)
			require.Equal(t, test.err, tracker.checkRootRejections())
		})
	}
}

// TestPromiseTrackerTracksLaterRootEvents updates root state after the temporary
// identity map has been discarded, while preserving independent rejection counts.
func TestPromiseTrackerTracksLaterRootEvents(t *testing.T) {
	vm := goja.New()
	tracker := NewPromiseTracker(vm)
	defer tracker.Close()
	entry := trackerEntry(t, vm, `() => {
		globalThis.root = new Promise((resolve, reject) => { globalThis.rejectRoot = reject; });
		return root;
	}`)

	// A root awaiting host work starts with no rejection and no retained capture map.
	root, err := tracker.callAsync(entry, goja.Undefined(), goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, goja.PromiseStatePending, root.State())
	require.Nil(t, tracker.initialUnhandled)
	require.Zero(t, tracker.unhandled)
	require.False(t, tracker.rootUnhandled)

	// A later root rejection is the one failure that result interpretation may consume.
	_, err = vm.RunString(`rejectRoot("private root");`)
	require.NoError(t, err)
	require.Equal(t, 1, tracker.unhandled)
	require.True(t, tracker.rootUnhandled)
	require.NoError(t, tracker.checkRootRejections())

	// Another rejected promise still blocks interpretation until it gains a handler.
	_, err = vm.RunString(`globalThis.detached = Promise.reject("private detached");`)
	require.NoError(t, err)
	require.Equal(t, 2, tracker.unhandled)
	require.Equal(t, ErrEvaluation, tracker.checkRootRejections())
	_, err = vm.RunString(`detached.catch(() => {});`)
	require.NoError(t, err)
	require.Equal(t, 1, tracker.unhandled)
	require.NoError(t, tracker.checkRootRejections())

	// Handling the root removes its exemption; a subsequent detached rejection cannot use it.
	_, err = vm.RunString(`root.catch(() => {});`)
	require.NoError(t, err)
	require.Zero(t, tracker.unhandled)
	require.False(t, tracker.rootUnhandled)
	_, err = vm.RunString(`Promise.reject("private later failure");`)
	require.NoError(t, err)
	require.Equal(t, 1, tracker.unhandled)
	require.Equal(t, ErrEvaluation, tracker.checkRootRejections())
	require.Nil(t, tracker.initialUnhandled)
}

// TestPromiseTrackerClearsFailedCapture releases temporary promise identities
// when invocation throws, returns an invalid root, or panics in trusted host code.
func TestPromiseTrackerClearsFailedCapture(t *testing.T) {
	for _, source := range []string{
		`() => { Promise.reject("private detached"); throw "private exception"; }`,
		`() => { Promise.reject("private detached"); return 42; }`,
	} {
		t.Run(source, func(t *testing.T) {
			vm := goja.New()
			tracker := NewPromiseTracker(vm)
			defer tracker.Close()
			entry := trackerEntry(t, vm, source)

			// Invalid calls fail safely and cannot leave the initial identity map reachable.
			root, err := tracker.callAsync(entry, goja.Undefined(), goja.Undefined())
			require.Equal(t, ErrEvaluation, err)
			require.Nil(t, root)
			require.Nil(t, tracker.root)
			require.Nil(t, tracker.initialUnhandled)
		})
	}

	t.Run("trusted panic", func(t *testing.T) {
		vm := goja.New()
		tracker := NewPromiseTracker(vm)
		defer tracker.Close()
		entry := func(goja.Value, ...goja.Value) (goja.Value, error) {
			_, err := vm.RunString(`Promise.reject("private detached");`)
			require.NoError(t, err)
			panic("trusted panic")
		}

		// Unexpected Go panics still propagate, with capture cleanup guaranteed by defer.
		require.PanicsWithValue(t, "trusted panic", func() {
			_, _ = tracker.callAsync(entry, goja.Undefined(), goja.Undefined())
		})
		require.Nil(t, tracker.initialUnhandled)
		require.Nil(t, tracker.root)
	})
}
