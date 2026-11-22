package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidationContextCancellation follows one caller through nested field and
// index paths while preserving the zero-value synchronous validation contract.
func TestValidationContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := &ValidationContext{Path: "$", Ctx: ctx}
	child := root.PushField("probes").PushIndex(2).PushField("if")

	// Derived paths retain caller identity without mutating the parent path.
	require.Equal(t, "$.probes[2].if", child.Path)
	require.Equal(t, "$", root.Path)
	require.Same(t, ctx, child.Context())
	cancel()
	require.ErrorIs(t, child.Context().Err(), context.Canceled)

	// Existing synchronous validation has no caller deadline, not a canceled one.
	require.NoError(t, (&ValidationContext{}).Context().Err())
	require.NoError(t, (*ValidationContext)(nil).Context().Err())
}

func TestValidationContext(t *testing.T) {
	ctx := &ValidationContext{}
	require.Equal(t, "", ctx.Path)

	orig := ctx
	ctx = ctx.PushField("field")
	require.Equal(t, "field", ctx.Path)
	require.Equal(t, "", orig.Path)

	orig = ctx
	ctx = ctx.PushIndex(0)
	require.Equal(t, "field[0]", ctx.Path)
	require.Equal(t, "field", orig.Path)

	ctx = ctx.PushField("field2")
	require.Equal(t, "field[0].field2: some error", ctx.NewError("some error").Error())
	require.Equal(t, "field[0].field2: some error value", ctx.NewErrorf("some error %v", "value").Error())
	require.Equal(t, "field[0].field2.field3: some error", ctx.NewErrorForField("field3", "some error").Error())
	require.Equal(t, "field[0].field2.field3: some error value", ctx.NewErrorfForField("field3", "some error %v", "value").Error())
}
