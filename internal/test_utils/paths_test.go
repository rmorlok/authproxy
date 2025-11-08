package test_utils

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestTestDataPath(t *testing.T) {
	path := TestDataPath("admin_user_keys/bobdole.pub")
	_, err := os.Stat(path)
	assert.NoError(t, err)
}
