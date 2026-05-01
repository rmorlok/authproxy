package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// NewBrowser starts a headless Chrome instance via chromedp and returns a
// context bound to it. The browser is shut down at test end.
//
// Tests that drive the OAuth flow through real UIs (marketplace + provider
// consent) use this helper. The 60s deadline guards against hangs in CI when
// the provider's HTML form has shifted shape.
func NewBrowser(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOpts...)

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)

	deadlineCtx, cancelDeadline := context.WithTimeout(browserCtx, 60*time.Second)

	cancel := func() {
		cancelDeadline()
		cancelBrowser()
		cancelAlloc()
	}
	t.Cleanup(cancel)

	return deadlineCtx, cancel
}
