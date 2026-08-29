package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEqualTimePointers(t *testing.T) {
	time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	assert.True(t, EqualTimePointers(nil, nil))
	assert.True(t, EqualTimePointers(&time.Time{}, &time.Time{}))
	assert.False(t, EqualTimePointers(&time.Time{}, nil))
	assert.False(t, EqualTimePointers(nil, &time.Time{}))
	assert.True(t, EqualTimePointers(
		ToPtr(time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)),
		ToPtr(time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC))),
	)
	assert.False(t, EqualTimePointers(
		ToPtr(time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)),
		ToPtr(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC))),
	)
}
