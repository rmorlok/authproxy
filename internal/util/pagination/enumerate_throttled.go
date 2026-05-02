package pagination

import (
	"context"

	"golang.org/x/time/rate"
)

// EnumerateFunc is the shape of an Enumerate method on a list builder. It
// matches the existing pagination iteration contract (a callback that
// receives one PageResult at a time) so any builder's Enumerate method
// can be passed as a method value.
type EnumerateFunc[T any] func(context.Context, func(PageResult[T]) (keepGoing KeepGoing, err error)) error

// EnumerateThrottled walks every item produced by enumerate, calling
// limiter.Wait(ctx) before invoking cb on each item. A nil limiter means
// no throttling. Stops on the first error from cb or limiter.Wait.
//
// Use this for background sweeps over large tables where the work per row
// is cheap and you want to bound throughput in records/sec rather than
// pause between fixed-size batches. The rate-limit check happens per row,
// not per page, so adjusting the page size doesn't change the throughput
// guarantee.
func EnumerateThrottled[T any](
	ctx context.Context,
	enumerate EnumerateFunc[T],
	limiter *rate.Limiter,
	cb func(T) error,
) error {
	return enumerate(ctx, func(pr PageResult[T]) (KeepGoing, error) {
		for _, item := range pr.Results {
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return Stop, err
				}
			}
			if err := cb(item); err != nil {
				return Stop, err
			}
		}
		return Continue, nil
	})
}
