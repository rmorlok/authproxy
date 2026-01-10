package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
