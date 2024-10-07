package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"strings"
)

func IsTokenExpiredError(err error) bool {
	if err == nil {
		return false
	}

	// The JWT package uses joined errors, which can be comma separated. Additionally, this might be wrapped.
	return strings.Contains(err.Error(), jwt.ErrTokenExpired.Error())
}
