package auth

// ClaimsUpdater defines interface adding extras to claims
type ClaimsUpdater interface {
	Update(claims JwtTokenClaims) JwtTokenClaims
}

// ClaimsUpdFunc type is an adapter to allow the use of ordinary functions as ClaimsUpdater. If f is a function
// with the appropriate signature, ClaimsUpdFunc(f) is a Handler that calls f.
type ClaimsUpdFunc func(claims JwtTokenClaims) JwtTokenClaims

// Update calls f(id)
func (f ClaimsUpdFunc) Update(claims JwtTokenClaims) JwtTokenClaims {
	return f(claims)
}
