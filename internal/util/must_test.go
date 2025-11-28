package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMust(t *testing.T) {
	t.Parallel()
	assert.True(t, Must(true, nil))
	assert.Panics(t, func() {
		Must(false, fmt.Errorf("I panic"))
	})
}

func TestMustNotError(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() {
		MustNotError(nil)
	})
	assert.Panics(t, func() {
		MustNotError(fmt.Errorf("I panic"))
	})
}
