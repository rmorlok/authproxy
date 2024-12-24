package auth

import "github.com/rmorlok/authproxy/jwt"

// Validator defines interface to accept o reject claims with consumer defined logic
// It works with valid token and allows to reject some, based on token match or user's fields
type Validator interface {
	Validate(token string, claims jwt.AuthProxyClaims) bool
}

// ValidatorFunc type is an adapter to allow the use of ordinary functions as Validator. If f is a function
// with the appropriate signature, ValidatorFunc(f) is a Validator that calls f.
type ValidatorFunc func(token string, claims jwt.AuthProxyClaims) bool

// Validate calls f(id)
func (f ValidatorFunc) Validate(token string, claims jwt.AuthProxyClaims) bool {
	return f(token, claims)
}
