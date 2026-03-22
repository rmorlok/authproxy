package jwt

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func Test_IsTokenExpiredError(t *testing.T) {
	t.Parallel()
	assert.False(t, IsTokenExpiredError(nil))
	assert.False(t, IsTokenExpiredError(jwt.ErrTokenMalformed))
	assert.True(t, IsTokenExpiredError(jwt.ErrTokenExpired))
	assert.True(t, IsTokenExpiredError(fmt.Errorf("some error: %w", jwt.ErrTokenExpired)))
	assert.True(t, IsTokenExpiredError(errors.New(strings.Join([]string{jwt.ErrTokenExpired.Error(), jwt.ErrTokenInvalidId.Error()}, ", "))))
}
