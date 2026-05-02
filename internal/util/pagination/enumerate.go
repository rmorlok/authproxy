package pagination

import "context"

// EnumerateCallback is the per-page callback shape passed to an
// Enumerate-style iterator. It returns Continue to fetch the next page,
// Stop to halt early, or an error to abort the iteration.
type EnumerateCallback[T any] func(PageResult[T]) (keepGoing KeepGoing, err error)

// EnumerateFunc is the shape of an Enumerate method on a list builder.
// Builders' Enumerate methods can be passed as values of this type
// directly via Go's method-value conversion, which lets generic helpers
// like EnumerateThrottled accept them without exposing the builder.
type EnumerateFunc[T any] func(context.Context, EnumerateCallback[T]) error
