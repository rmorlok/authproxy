package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func Test_IsTokenExpiredError(t *testing.T) {
	assert.False(t, IsTokenExpiredError(nil))
	assert.False(t, IsTokenExpiredError(jwt.ErrTokenMalformed))
	assert.True(t, IsTokenExpiredError(jwt.ErrTokenExpired))
	assert.True(t, IsTokenExpiredError(errors.Wrap(jwt.ErrTokenExpired, "some error")))
	assert.True(t, IsTokenExpiredError(errors.New(strings.Join([]string{jwt.ErrTokenExpired.Error(), jwt.ErrTokenInvalidId.Error()}, ", "))))
}
