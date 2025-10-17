package util

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	assert.Equal(t, []int{2, 3, 4}, Map([]string{"1", "2", "3"}, func(s string) int {
		return Must(strconv.Atoi(s)) + 1
	}))
}

func TestGetKeys(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, GetKeys(map[string]int{"a": 1, "b": 2, "c": 3}))

	var m map[string]int
	assert.Equal(t, []string{}, GetKeys(m))
}
