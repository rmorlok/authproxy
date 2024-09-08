package auth

import (
	"github.com/rmorlok/authproxy/logger"
	"net/http"
	"time"
)

const (
	// default names for cookies and headers
	defaultJWTCookieName   = "JWT"
	defaultJWTCookieDomain = ""
	defaultJWTHeaderKey    = "X-JWT"
	defaultXSRFCookieName  = "XSRF-TOKEN"
	defaultXSRFHeaderKey   = "X-XSRF-TOKEN"
	defaultTokenQuery      = "token"

	// We don't normally issues tokens ourselves except through CLI tools for testing.
	defaultIssuer         = "authproxy/auth"
	defaultTokenDuration  = time.Minute * 15
	defaultCookieDuration = time.Hour * 24 * 31
)

// Opts holds constructor params
type Opts struct {
	SecretReader   SecretReader
	ClaimsUpd      ClaimsUpdater
	SecureCookies  bool
	TokenDuration  time.Duration
	CookieDuration time.Duration
	DisableXSRF    bool
	DisableIAT     bool // disable IssuedAt claim
	// optional (custom) names for cookies and headers
	JWTCookieName   string
	JWTCookieDomain string
	JWTHeaderKey    string
	XSRFCookieName  string
	XSRFHeaderKey   string
	JWTQuery        string
	AudienceReader  Audience      // allowed aud values
	Issuer          string        // optional value for iss claim, usually application name
	AudSecrets      bool          // uses different secret for differed auds. important: adds pre-parsing of unverified token
	SendJWTHeader   bool          // if enabled send JWT as a header instead of cookie
	SameSite        http.SameSite // define a cookie attribute making it impossible for the browser to send this cookie cross-site

	Logger       logger.L // logger interface, default is no logging at all
	RefreshCache RefreshCache
	Validator    Validator
}
