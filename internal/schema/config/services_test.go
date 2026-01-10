package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestServiceIds(t *testing.T) {
	t.Run("AllServiceIds", func(t *testing.T) {
		t.Run("has values", func(t *testing.T) {
			assert.True(t, len(AllServiceIds()) > 0)
		})
	})
	t.Run("IsValidServiceId", func(t *testing.T) {
		assert.True(t, IsValidServiceId(ServiceIdAdminApi))
		assert.False(t, IsValidServiceId(""))
		assert.False(t, IsValidServiceId("bad"))
	})
	t.Run("AllValidServiceIds", func(t *testing.T) {
		assert.True(t, AllValidServiceIds([]string{}))
		assert.True(t, AllValidServiceIds([]string{string(ServiceIdAdminApi)}))
		assert.False(t, IsValidServiceId("bad"))
	})
}
