package util

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestMap(t *testing.T) {
	assert.Equal(t, []int{2, 3, 4}, Map([]string{"1", "2", "3"}, func(s string) int {
		return Must(strconv.Atoi(s)) + 1
	}))
}
