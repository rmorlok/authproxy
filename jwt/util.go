package jwt

import "github.com/golang-jwt/jwt/v5"

// ClaimString converts a singular string into a claims string.
func ClaimString(s string) jwt.ClaimStrings {
	return jwt.ClaimStrings{s}
}
