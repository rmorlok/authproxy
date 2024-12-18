package auth

import (
	"fmt"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

func signingKeyData(key config.Key) config.KeyData {
	switch v := key.(type) {
	case *config.KeyPublicPrivate:
		return v.PrivateKey
	case *config.KeyShared:
		return v.SharedKey
	default:
		panic("key does not support signing")
	}
}

func verificationKeyData(key config.Key) config.KeyData {
	switch v := key.(type) {
	case *config.KeyPublicPrivate:
		return v.PublicKey
	case *config.KeyShared:
		return v.SharedKey
	default:
		panic("key does not support verification")
	}
}

// keyDataForToken loads an appropriate key to sign or verify a given token. This accounts for the
// fact that admin users will verify with different keys to sign/verify tokens.
func (s *Service) keyForToken(claims *JwtTokenClaims) (config.Key, error) {
	if claims.IsAdmin() {
		adminUsername, err := claims.AdminUsername()
		if err != nil {
			return nil, errors.Wrap(err, "failed to establish admin username to sign jwt")
		}

		adminUser, ok := s.Config.GetRoot().SystemAuth.AdminUsers.GetByUsername(adminUsername)
		if !ok {
			return nil, errors.Errorf("admin user '%s' not found", adminUsername)
		}

		return adminUser.Key, nil
	} else {
		return s.Config.GetRoot().SystemAuth.JwtSigningKey, nil
	}
}

// Token mints a signed JWT with the specified claims
func (s *Service) Token(ctx context.Context, claims *JwtTokenClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	audiences, err := claims.GetAudience()
	if err != nil {
		return "", errors.Wrap(err, "failed to get aud")
	}

	if !config.AllValidServiceIds(audiences) {
		return "", errors.Wrap(err, "some service ids in aud are invalid")
	}

	key, err := s.keyForToken(claims)
	if err != nil {
		return "", errors.Wrap(err, "failed to get key")
	}

	if !key.CanSign() {
		return "", errors.New("key cannot be used to sign tokens")
	}

	keyData := signingKeyData(key)

	if !keyData.HasData(ctx) {
		return "", errors.New("no data found in signing key")
	}

	secret, err := keyData.GetData(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get key data for signing")
	}

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", errors.Wrap(err, "can't sign token")
	}
	return tokenString, nil
}

// Parse token string and verify.
func (s *Service) Parse(ctx context.Context, tokenString string) (*JwtTokenClaims, error) {
	parser := jwt.NewParser(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	// Now parse with verification, using the key for this actor
	token, err := parser.ParseWithClaims(tokenString, &JwtTokenClaims{}, func(unverified *jwt.Token) (interface{}, error) {
		isSecretKey := false
		switch unverified.Method.(type) {
		case *jwt.SigningMethodHMAC:
			isSecretKey = true
		case *jwt.SigningMethodRSA:
			isSecretKey = false
		default:
			return nil, errors.Errorf("unexpected signing method: %v", unverified.Header["alg"])
		}

		unverifiedClaims, ok := unverified.Claims.(*JwtTokenClaims)
		if !ok {
			return nil, errors.New("invalid token")
		}

		key, err := s.keyForToken(unverifiedClaims)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get key")
		}

		if !key.CanVerifySignature() {
			return nil, errors.New("key cannot be used to verify signatures")
		}

		keyData := verificationKeyData(key)

		if !keyData.HasData(ctx) {
			return nil, errors.New("no data found in signing key")
		}

		rawKeyData, err := keyData.GetData(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get key data for signing")
		}

		if isSecretKey {
			return rawKeyData, nil
		}

		return loadRSAPublicKeyFromPEMOrOpenSSH(rawKeyData)
	})
	if err != nil {
		return nil, errors.Wrap(err, "can't parse token")
	}

	claims, ok := token.Claims.(*JwtTokenClaims)
	if !ok {
		return nil, errors.New("invalid token")
	}

	found := false
	for _, aud := range claims.Audience {
		if aud == string(s.Opts.ServiceId) {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.Errorf("aud not valid for service '%s'", s.Opts.ServiceId)
	}

	return claims, s.validate(ctx, claims)
}

func (s *Service) validate(ctx context.Context, claims *JwtTokenClaims) error {
	v := jwt.NewValidator(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	return v.Validate(claims)
}

func (s *Service) setJwtResponseHeader(w http.ResponseWriter, tokenString string) {
	w.Header().Set(jwtHeaderKey, fmt.Sprintf("Bearer %s", tokenString))
}

// Set creates token cookie with xsrf cookie and put it to ResponseWriter
// accepts claims and sets expiration if none defined. permanent flag means long-living cookie,
// false makes it session only.
func (s *Service) Set(ctx context.Context, w http.ResponseWriter, claims *JwtTokenClaims) (*JwtTokenClaims, error) {
	expiresAt, err := claims.GetExpirationTime()
	if err != nil {
		return nil, errors.Wrap(err, "can't get expiration time")
	}

	if expiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(ctx.Clock().Now().Add(s.Config.GetRoot().SystemAuth.JwtTokenDuration()))
	}

	claims.Issuer = string(s.ServiceId)
	claims.IssuedAt = jwt.NewNumericDate(ctx.Clock().Now())

	tokenString, err := s.Token(ctx, claims)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make token token")
	}

	if s.SendJWTHeader {
		s.setJwtResponseHeader(w, tokenString)
		return claims, nil
	}

	cookieExpiration := 0 // session cookie
	if !claims.SessionOnly {
		cookieExpiration = int(s.Config.GetRoot().SystemAuth.CookieDuration().Seconds())
	}

	jwtCookie := http.Cookie{
		Name:     jwtCookieName,
		Value:    tokenString,
		HttpOnly: true,
		Path:     "/",
		Domain:   s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   s.apiHost().IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{
		Name:     xsrfCookieName,
		Value:    claims.ID,
		HttpOnly: false,
		Path:     "/",
		Domain:   s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   s.apiHost().IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &xsrfCookie)

	return claims, nil
}

// extractTokenFromBearer extracts the token v
func extractTokenFromBearer(authorizationHeader string) (string, error) {
	if strings.HasPrefix(authorizationHeader, "Bearer ") {
		return strings.TrimPrefix(authorizationHeader, "Bearer "), nil
	}

	return "", errors.New("no bearer token found")
}

// Get token from url, header, or cookie
// if cookie used, verify xsrf token to match
func (s *Service) Get(ctx context.Context, r *http.Request) (*JwtTokenClaims, string, error) {

	fromCookie := false
	tokenString := ""

	// try to get from "token" query param
	if tkQuery := r.URL.Query().Get(jwtQueryParam); tkQuery != "" {
		tokenString = tkQuery
	}

	// try to get from JWT header
	if tokenHeader := r.Header.Get(jwtHeaderKey); tokenHeader != "" && tokenString == "" {
		var err error
		tokenString, err = extractTokenFromBearer(tokenHeader)
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to extract token from authorization header")
		}
	}

	// try to get from JWT cookie
	if tokenString == "" {
		fromCookie = true
		jc, err := r.Cookie(jwtCookieName)
		if err != nil {
			return nil, "", errors.Wrap(err, "token cookie was not presented")
		}
		tokenString = jc.Value
	}

	claims, err := s.Parse(ctx, tokenString)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get token")
	}

	// promote claim's aud to User.Audience
	if claims.Actor != nil {
		claims.Actor.Audience = claims.Audience
	}

	if s.Config.GetRoot().SystemAuth.DisableXSRF {
		return claims, tokenString, nil
	}

	if fromCookie && claims.Actor != nil {
		xsrf := r.Header.Get(xsrfHeaderKey)
		if claims.ID != xsrf {
			return nil, "", errors.New("xsrf mismatch")
		}
	}

	return claims, tokenString, nil
}

// IsExpired returns true if claims expired
func (s *Service) IsExpired(ctx context.Context, claims *JwtTokenClaims) bool {
	return claims.ExpiresAt != nil && claims.ExpiresAt.Before(ctx.Clock().Now())
}

// Reset token's cookies
func (s *Service) Reset(w http.ResponseWriter) {
	jwtCookie := http.Cookie{Name: jwtCookieName, Value: "", HttpOnly: false, Path: "/", Domain: s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: s.apiHost().IsHttps(), SameSite: cookieSameSite}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{Name: xsrfCookieName, Value: "", HttpOnly: false, Path: "/", Domain: s.Config.GetRoot().SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: s.apiHost().IsHttps(), SameSite: cookieSameSite}
	http.SetCookie(w, &xsrfCookie)
}

// claimStringsVal returns a string for used in errors/logging for claims string that accounts for the fact
// that often it will be a single string and we don't need to print an array when that is the case.
func claimStringsVal(cs jwt.ClaimStrings) string {
	if len(cs) == 0 {
		return "''"
	}

	if len(cs) == 1 {
		return cs[0]
	}

	return fmt.Sprintf("%q", cs)
}

// ClaimString converts a singular string into a claims string.
func ClaimString(s string) jwt.ClaimStrings {
	return jwt.ClaimStrings{s}
}
