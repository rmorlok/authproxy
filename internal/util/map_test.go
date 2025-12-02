package util

import (
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []int{2, 3, 4}, Map([]string{"1", "2", "3"}, func(s string) int {
		return Must(strconv.Atoi(s)) + 1
	}))
}

func TestGetKeys(t *testing.T) {
	t.Parallel()
	result := GetKeys(map[string]int{"a": 1, "b": 2, "c": 3})
	slices.Sort(result)
	assert.Equal(t, []string{"a", "b", "c"}, result)

	var m map[string]int
	assert.Equal(t, []string{}, GetKeys(m))
}

func TestFlatMap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []int{1, 2, 3}, FlatMap([][]int{{1, 2}, {3}}, func(i []int) []int { return i }))
	assert.Equal(t, []int{}, FlatMap([][]int{}, func(i []int) []int { return i }))
	assert.Equal(t, []int{}, FlatMap(nil, func(i []int) []int { return i }))
}
