package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMax(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3, MaxInt(1, 2, 3))
	assert.Equal(t, 1, MaxInt(1))
	assert.Equal(t, 3, MaxInt(3, 2))
	assert.Panics(t, func() { MaxInt() })

	assert.Equal(t, int32(3), MaxInt32(int32(1), int32(2), int32(3)))
	assert.Equal(t, int32(1), MaxInt32(int32(1)))
	assert.Equal(t, int32(3), MaxInt32(int32(3), int32(2)))
	assert.Panics(t, func() { MaxInt32() })

	assert.Equal(t, int64(3), MaxInt64(int64(1), int64(2), int64(3)))
	assert.Equal(t, int64(1), MaxInt64(int64(1)))
	assert.Equal(t, int64(3), MaxInt64(int64(3), int64(2)))
	assert.Panics(t, func() { MaxInt64() })

	assert.Equal(t, uint64(3), MaxUint64(uint64(1), uint64(2), uint64(3)))
	assert.Equal(t, uint64(1), MaxUint64(uint64(1)))
	assert.Equal(t, uint64(3), MaxUint64(uint64(3), uint64(2)))
	assert.Panics(t, func() { MaxUint64() })
}

func TestMin(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, MinInt(1, 2, 3))
	assert.Equal(t, 1, MinInt(1))
	assert.Equal(t, 2, MinInt(3, 2))
	assert.Panics(t, func() { MinInt() })

	assert.Equal(t, int32(1), MinInt32(int32(1), int32(2), int32(3)))
	assert.Equal(t, int32(1), MinInt32(int32(1)))
	assert.Equal(t, int32(2), MinInt32(int32(3), int32(2)))
	assert.Panics(t, func() { MinInt32() })

	assert.Equal(t, int64(1), MinInt64(int64(1), int64(2), int64(3)))
	assert.Equal(t, int64(1), MinInt64(int64(1)))
	assert.Equal(t, int64(2), MinInt64(int64(3), int64(2)))
	assert.Panics(t, func() { MinInt64() })

	assert.Equal(t, uint64(1), MinUint64(uint64(1), uint64(2), uint64(3)))
	assert.Equal(t, uint64(1), MinUint64(uint64(1)))
	assert.Equal(t, uint64(2), MinUint64(uint64(3), uint64(2)))
	assert.Panics(t, func() { MinUint64() })
}
