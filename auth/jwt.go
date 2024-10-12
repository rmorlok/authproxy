// Package token wraps jwt-go library and provides higher level abstraction to work with JWT.
package auth

import (
	"encoding/json"
	"fmt"
	"github.com/rmorlok/authproxy/context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

// Claims stores actor info for token and state & from login
type Claims struct {
	jwt.RegisteredClaims
	Actor       *Actor `json:"user,omitempty"`
	SessionOnly bool   `json:"sess_only,omitempty"`
}

// Token mints a signed JWT with the specified claims
func (j *Service) Token(claims Claims) (string, error) {

	// make token for allowed aud values only, rejects others

	// update claims with ClaimsUpdFunc defined by consumer
	if j.ClaimsUpd != nil {
		claims = j.ClaimsUpd.Update(claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	if j.SecretReader == nil {
		return "", errors.New("secret reader not defined")
	}

	if err := j.checkAuds(&claims, j.AudienceReader); err != nil {
		return "", errors.Wrap(err, "aud rejected")
	}

	audiences, err := claims.GetAudience()
	if err != nil {
		return "", errors.Wrap(err, "failed to get aud")
	}

	var secret string
	for _, aud := range audiences {
		secret, err = j.SecretReader.GetForAudience(aud) // get secret via consumer defined SecretReader
		if err == nil {
			break
		}
	}

	if err != nil {
		return "", errors.Wrap(err, "can't get secret")
	}

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", errors.Wrap(err, "can't sign token")
	}
	return tokenString, nil
}

// Parse token string and verify.
func (j *Service) Parse(ctx context.Context, tokenString string) (Claims, error) {
	parser := jwt.NewParser(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	if j.SecretReader == nil {
		return Claims{}, errors.New("secret reader not defined")
	}

	var err error

	audiences := []string{"ignore"}
	if j.AudSecrets {
		audiences, err = j.aud(tokenString)
		if err != nil {
			return Claims{}, errors.New("can't retrieve audience from the token")
		}
	}

	var secret string
	for _, aud := range audiences {
		secret, err = j.SecretReader.GetForAudience(aud) // get secret via consumer defined SecretReader
		if err == nil {
			break
		}
	}

	if err != nil {
		return Claims{}, errors.Wrap(err, "can't get secret")
	}

	token, err := parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return Claims{}, errors.Wrap(err, "can't parse token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return Claims{}, errors.New("invalid token")
	}

	if err = j.checkAuds(claims, j.AudienceReader); err != nil {
		return Claims{}, errors.Wrap(err, "aud rejected")
	}
	return *claims, j.validate(ctx, claims)
}

// aud pre-parse token and extracts aud from the claim
// important! this step ignores token verification, should not be used for any validations
func (j *Service) aud(tokenString string) ([]string, error) {
	parser := jwt.NewParser(
		jwt.WithoutClaimsValidation(),
	)
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, errors.Wrap(err, "can't pre-parse token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid token")
	}

	aud, err := claims.GetAudience()
	if err != nil {
		return nil, errors.Wrap(err, "can't get audience")
	}

	if len(aud) == 0 {
		return nil, errors.New("empty aud")
	}
	return claims.Audience, nil
}

func (j *Service) validate(ctx context.Context, claims *Claims) error {
	v := jwt.NewValidator(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	return v.Validate(claims)
}

// Set creates token cookie with xsrf cookie and put it to ResponseWriter
// accepts claims and sets expiration if none defined. permanent flag means long-living cookie,
// false makes it session only.
func (j *Service) Set(ctx context.Context, w http.ResponseWriter, claims Claims) (Claims, error) {
	expiresAt, err := claims.GetExpirationTime()
	if err != nil {
		return Claims{}, errors.Wrap(err, "can't get expiration time")
	}

	if expiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(ctx.Clock().Now().Add(j.Config.SystemAuth.JwtTokenDuration()))
	}

	if claims.Issuer == "" {
		claims.Issuer = j.Config.SystemAuth.JwtIssuer()
	}

	claims.IssuedAt = jwt.NewNumericDate(ctx.Clock().Now())

	tokenString, err := j.Token(claims)
	if err != nil {
		return Claims{}, errors.Wrap(err, "failed to make token token")
	}

	if j.SendJWTHeader {
		w.Header().Set(jwtHeaderKey, tokenString)
		return claims, nil
	}

	cookieExpiration := 0 // session cookie
	if !claims.SessionOnly {
		cookieExpiration = int(j.Config.SystemAuth.CookieDuration().Seconds())
	}

	jwtCookie := http.Cookie{
		Name:     jwtCookieName,
		Value:    tokenString,
		HttpOnly: true,
		Path:     "/",
		Domain:   j.Config.SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   j.ApiHost.IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{
		Name:     xsrfCookieName,
		Value:    claims.ID,
		HttpOnly: false,
		Path:     "/",
		Domain:   j.Config.SystemAuth.CookieDomain,
		MaxAge:   cookieExpiration,
		Secure:   j.ApiHost.IsHttps(),
		SameSite: cookieSameSite,
	}
	http.SetCookie(w, &xsrfCookie)

	return claims, nil
}

// Get token from url, header or cookie
// if cookie used, verify xsrf token to match
func (j *Service) Get(ctx context.Context, r *http.Request) (Claims, string, error) {

	fromCookie := false
	tokenString := ""

	// try to get from "token" query param
	if tkQuery := r.URL.Query().Get(jwtQueryParam); tkQuery != "" {
		tokenString = tkQuery
	}

	// try to get from JWT header
	if tokenHeader := r.Header.Get(jwtHeaderKey); tokenHeader != "" && tokenString == "" {
		tokenString = tokenHeader
	}

	// try to get from JWT cookie
	if tokenString == "" {
		fromCookie = true
		jc, err := r.Cookie(jwtCookieName)
		if err != nil {
			return Claims{}, "", errors.Wrap(err, "token cookie was not presented")
		}
		tokenString = jc.Value
	}

	claims, err := j.Parse(ctx, tokenString)
	if err != nil {
		return Claims{}, "", errors.Wrap(err, "failed to get token")
	}

	// promote claim's aud to User.Audience
	if claims.Actor != nil {
		claims.Actor.Audience = claims.Audience
	}

	if j.Config.SystemAuth.DisableXSRF {
		return claims, tokenString, nil
	}

	if fromCookie && claims.Actor != nil {
		xsrf := r.Header.Get(xsrfHeaderKey)
		if claims.ID != xsrf {
			return Claims{}, "", errors.New("xsrf mismatch")
		}
	}

	return claims, tokenString, nil
}

// IsExpired returns true if claims expired
func (j *Service) IsExpired(ctx context.Context, claims Claims) bool {
	return claims.ExpiresAt != nil && claims.ExpiresAt.Before(ctx.Clock().Now())
}

// Reset token's cookies
func (j *Service) Reset(w http.ResponseWriter) {
	jwtCookie := http.Cookie{Name: jwtCookieName, Value: "", HttpOnly: false, Path: "/", Domain: j.Config.SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: j.ApiHost.IsHttps(), SameSite: cookieSameSite}
	http.SetCookie(w, &jwtCookie)

	xsrfCookie := http.Cookie{Name: xsrfCookieName, Value: "", HttpOnly: false, Path: "/", Domain: j.Config.SystemAuth.CookieDomain,
		MaxAge: -1, Expires: time.Unix(0, 0), Secure: j.ApiHost.IsHttps(), SameSite: cookieSameSite}
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

// checkAuds verifies if claims.Audience in the list of allowed by audReader
func (j *Service) checkAuds(claims *Claims, audReader Audience) error {
	if audReader == nil { // lack of any allowed means any
		return nil
	}
	auds, err := audReader.Get()
	if err != nil {
		return errors.Wrap(err, "failed to get auds")
	}
	for _, a := range auds {
		for _, claimAud := range claims.Audience {
			if strings.EqualFold(a, claimAud) {
				return nil
			}
		}
	}
	return errors.Errorf("aud %s not allowed", claimStringsVal(claims.Audience))
}

func (c Claims) String() string {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Sprintf("%+v %+v", c.RegisteredClaims, c.Actor)
	}
	return string(b)
}

// SecretReader defines interface returning secret key for given id (aud)
type SecretReader interface {
	GetForAudience(aud string) (string, error) // aud matching is optional. Implementation may decide if supported or ignored
}

// SecretFunc type is an adapter to allow the use of ordinary functions as Secret. If f is a function
// with the appropriate signature, SecretFunc(f) is a Handler that calls f.
type SecretFunc func(aud string) (string, error)

// GetForAudience calls f()
func (f SecretFunc) GetForAudience(aud string) (string, error) {
	return f(aud)
}

// Audience defines interface returning list of allowed audiences
type Audience interface {
	Get() ([]string, error)
}

// AudienceFunc type is an adapter to allow the use of ordinary functions as Audience.
type AudienceFunc func() ([]string, error)

// Get calls f()
func (f AudienceFunc) Get() ([]string, error) {
	return f()
}

// ClaimsString converts a singular string into a claims string.
func ClaimString(s string) jwt.ClaimStrings {
	return jwt.ClaimStrings{s}
}
