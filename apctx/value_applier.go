package apctx

import "context"

type valueApplier struct {
	key   string
	value interface{}
}

func (va *valueApplier) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, va.key, va.value)
}

// Set allows you to take an arbitrary key and value and use it in With(...) chaining on the context.
//
// e.g. ctx := context.Context().
//
//	With(util.Set("dog", "woof")).
//	With(util.Set("cat", "meow"))
func Set(key string, value interface{}) WithApplier {
	return &valueApplier{key, value}
}
