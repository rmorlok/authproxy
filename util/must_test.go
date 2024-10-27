package util

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMust(t *testing.T) {
	assert.True(t, Must(true, nil))
	assert.Panics(t, func() {
		Must(false, fmt.Errorf("I panic"))
	})
}

func TestMustNotError(t *testing.T) {
	assert.NotPanics(t, func() {
		MustNotError(nil)
	})
	assert.Panics(t, func() {
		MustNotError(fmt.Errorf("I panic"))
	})
}
